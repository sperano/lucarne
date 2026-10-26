// Package stream holds the live state of the runs being viewed and serves
// it as snapshots and Server-Sent Event streams, as contract/README.md
// specifies.
//
// # One history per run
//
// The first request for a run resolves it in the source, lists its
// attempts and starts following them: one goroutine polls the Task and its
// attempts, one per attempt reads its log through the runner's adapter.
// Every change is an update with the run's next seq, encoded once and kept
// in a bounded replay buffer; transcript events are also retained, bounded,
// for snapshots. All of it happens under the run's lock, so a snapshot
// reflects exactly the updates up to its lastSeq and a subscription from
// that cursor gets exactly the later ones: neither misses nor repeats an
// update. The history is named by a random epoch: a server restart, or a
// run evicted after it went unwatched, starts another one, and a cursor of
// the old epoch gets a reset.
//
// # Reading a log again
//
// A read that ends unavailable is retried from the start of the log, with
// the origin already read as KnownOrigin. Its lines up to the last one
// delivered are the overlap and are skipped: the re-read is lined up on
// that last line, found again by its timestamp and content, so two
// identical lines stay two lines. A re-read that cannot be lined up, or
// whose log starts with another line, is a gap, never a silent overlap.
//
// # Bounds and backpressure
//
// Updates wait for a slow client in a queue of MaxClientQueue; one that
// overflows gets a client_too_slow reset and is released. A client gone,
// a run finished or deleted, or the hub closed releases its stream. A run
// nobody watched for an idle timeout is dropped with all it holds, and the
// hub follows a bounded number of runs.
package stream
