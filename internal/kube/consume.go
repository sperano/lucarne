package kube

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// consume delivers the lines of one stream, past those a resumed stream
// reads again. It reports whether the read is over.
func (rd *reader) consume(ctx context.Context, lines *lineReader, inst instance, follow bool) (bool, error) {
	var overlap *resume
	if !rd.tail.empty() {
		overlap = newResume(&rd.tail)
	}
	for {
		l, err := lines.next()
		if err != nil {
			return rd.streamEnded(ctx, err, inst, follow, overlap != nil)
		}
		if !l.ended && (follow || !inst.terminated) {
			// A line the stream cut is read again by the next stream.
			continue
		}
		if overlap != nil {
			step := overlap.step(l)
			if step != overlapLost {
				if step == overlapFound {
					overlap = nil
				}
				continue
			}
			overlap = nil
			if err := rd.gap(contract.Gap{Reason: contract.GapReadInterrupted, Detail: detailNotFoundAgain}); err != nil {
				return true, err
			}
		}
		if done, err := rd.deliver(l); done || err != nil {
			return true, err
		}
	}
}

// detailNotFoundAgain is the detail of a gap a reconnect could not close.
const detailNotFoundAgain = "the log stream broke and its last lines were not found again"

// streamEnded handles the end of one stream: the end of the log, of what
// it holds so far, or a break. A log that ends before a resumed stream
// found its last lines again cannot prove that none are missing.
func (rd *reader) streamEnded(ctx context.Context, err error, inst instance, follow, unresolved bool) (bool, error) {
	switch {
	case ctx.Err() != nil:
		return true, ctx.Err()
	case !errors.Is(err, io.EOF):
		return rd.broke(ctx)
	case follow:
		// The instance terminated, or the server closed the stream: check
		// the instance again, and read the rest of its log.
		return false, nil
	}
	if unresolved {
		if err := rd.gap(contract.Gap{Reason: contract.GapReadInterrupted, Detail: detailNotFoundAgain}); err != nil {
			return true, err
		}
	}
	if inst.terminated {
		return true, rd.end(ctx, EndComplete, "")
	}
	return true, rd.end(ctx, EndCurrent, "")
}

// broke counts a stream that opened and broke with no new line since the
// last break; after maxQuietBreaks in a row the log is reported
// unavailable.
func (rd *reader) broke(ctx context.Context) (bool, error) {
	if rd.res.Lines > rd.brokenAt {
		rd.breaks, rd.brokenAt = 0, rd.res.Lines
		return false, nil
	}
	if rd.breaks++; rd.breaks >= maxQuietBreaks {
		return true, rd.end(ctx, EndUnavailable, ReasonStreamBreaks)
	}
	return false, nil
}

// deliver hands the next line to the sink, unless it takes the log past
// contract.MaxAttemptLogBytes. It reports whether the read is over.
func (rd *reader) deliver(l rawLine) (bool, error) {
	if rd.res.Bytes+l.size > contract.MaxAttemptLogBytes {
		rd.res.End, rd.res.Reason = EndLimit, ReasonInputLimit
		if rd.res.Origin == "" {
			return true, nil
		}
		return true, rd.gap(contract.Gap{Reason: contract.GapInputLimit})
	}
	if rd.res.Lines == 0 {
		rd.res.Origin = l.origin()
		if known := rd.opts.KnownOrigin; known != "" && known != rd.res.Origin {
			if err := rd.gap(contract.Gap{Reason: contract.GapSourceTruncated, Detail: detailOtherOrigin}); err != nil {
				return true, err
			}
		}
	}
	rd.res.Lines++
	rd.res.Bytes += l.size
	if l.bytes > contract.MaxLineBytes {
		rd.res.TruncatedLines++
	}
	rd.tail.add(l)
	line := adapter.Line{Number: rd.res.Lines, Text: l.text, Bytes: l.bytes}
	if l.time != nil {
		line.LogTime = &contract.Timestamp{Time: *l.time}
	}
	return false, rd.sink.Line(rd.res.Origin, line)
}

// detailOtherOrigin is the detail of a gap found when a log read again
// starts with another line than before.
const detailOtherOrigin = "the log starts with another line than when it was read before"

// wait sleeps before a follow tries again, or returns the context's error.
func (s *Source) wait(ctx context.Context) error {
	if s.retryDelay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(s.retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
