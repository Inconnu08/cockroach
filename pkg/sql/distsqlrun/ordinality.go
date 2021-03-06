// Copyright 2019 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package distsqlrun

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/sql/distsqlpb"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/sqlbase"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
	"github.com/cockroachdb/cockroach/pkg/util/tracing"
	"github.com/opentracing/opentracing-go"
)

// ordinalityProcessor is the processor of the WITH ORDINALITY operator, which
// adds an additional ordinal column to the result.
type ordinalityProcessor struct {
	ProcessorBase

	input  RowSource
	curCnt int64
}

var _ Processor = &ordinalityProcessor{}
var _ RowSource = &ordinalityProcessor{}

const ordinalityProcName = "ordinality"

func newOrdinalityProcessor(
	flowCtx *FlowCtx,
	processorID int32,
	spec *distsqlpb.OrdinalitySpec,
	input RowSource,
	post *distsqlpb.PostProcessSpec,
	output RowReceiver,
) (RowSourcedProcessor, error) {
	ctx := flowCtx.EvalCtx.Ctx()
	o := &ordinalityProcessor{input: input, curCnt: 1}

	colTypes := make([]types.T, len(input.OutputTypes())+1)
	copy(colTypes, input.OutputTypes())
	colTypes[len(colTypes)-1] = *types.Int
	if err := o.Init(
		o,
		post,
		colTypes,
		flowCtx,
		processorID,
		output,
		nil, /* memMonitor */
		ProcStateOpts{
			InputsToDrain: []RowSource{o.input},
			TrailingMetaCallback: func(context.Context) []distsqlpb.ProducerMetadata {
				o.ConsumerClosed()
				return nil
			}},
	); err != nil {
		return nil, err
	}

	if sp := opentracing.SpanFromContext(ctx); sp != nil && tracing.IsRecording(sp) {
		o.input = NewInputStatCollector(o.input)
		o.finishTrace = o.outputStatsToTrace
	}

	return o, nil
}

// Start is part of the RowSource interface.
func (o *ordinalityProcessor) Start(ctx context.Context) context.Context {
	o.input.Start(ctx)
	return o.StartInternal(ctx, ordinalityProcName)
}

// Next is part of the RowSource interface.
func (o *ordinalityProcessor) Next() (sqlbase.EncDatumRow, *distsqlpb.ProducerMetadata) {
	for o.State == StateRunning {
		row, meta := o.input.Next()

		if meta != nil {
			if meta.Err != nil {
				o.MoveToDraining(nil /* err */)
			}
			return nil, meta
		}
		if row == nil {
			o.MoveToDraining(nil /* err */)
			break
		}

		// The ordinality should increment even if the row gets filtered out.
		row = append(row, sqlbase.DatumToEncDatum(types.Int, tree.NewDInt(tree.DInt(o.curCnt))))
		o.curCnt++
		if outRow := o.ProcessRowHelper(row); outRow != nil {
			return outRow, nil
		}
	}
	return nil, o.DrainHelper()

}

// ConsumerClosed is part of the RowSource interface.
func (o *ordinalityProcessor) ConsumerClosed() {
	// The consumer is done, Next() will not be called again.
	o.InternalClose()
}

const ordinalityTagPrefix = "ordinality."

// Stats implements the SpanStats interface.
func (os *OrdinalityStats) Stats() map[string]string {
	return os.InputStats.Stats(ordinalityTagPrefix)
}

// StatsForQueryPlan implements the DistSQLSpanStats interface.
func (os *OrdinalityStats) StatsForQueryPlan() []string {
	return os.InputStats.StatsForQueryPlan("")
}

// outputStatsToTrace outputs the collected distinct stats to the trace. Will
// fail silently if the Distinct processor is not collecting stats.
func (o *ordinalityProcessor) outputStatsToTrace() {
	is, ok := getInputStats(o.flowCtx, o.input)
	if !ok {
		return
	}
	if sp := opentracing.SpanFromContext(o.Ctx); sp != nil {
		tracing.SetSpanStats(
			sp, &OrdinalityStats{InputStats: is},
		)
	}
}
