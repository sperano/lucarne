// Package kube is Lucarne's Kubernetes source: it resolves a run key to a
// Kelos Task, lists the attempts of its agent container and reads their
// logs, bounded and verified. It only ever gets, lists and reads logs; it
// never writes to the cluster, and its credentials stay in the server.
//
// # Identity
//
// A run is a Task found by UID in an allowlisted namespace of the
// configured cluster; any other cluster or namespace is "not found" without
// a call to the API. A Task is listed and matched by UID, never fetched by
// a caller-chosen name. Its pods are the ones whose controller is the Job
// whose controller is that Task, both by UID (Kelos 0.60.0 names the Job
// after the Task and labels its pods kelos.dev/task=<task name>, so a name
// alone would also find a reused name's objects). Before each log read the
// whole chain is checked again, and the pod's UID once more after the
// stream is open, so a read never reaches another Task's pod.
//
// # Attempts
//
// An attempt is one instance of the pod's agent container (kelos-agent),
// identified by pod UID, container and restart count. Init containers and
// user extra containers are never read. Kubernetes keeps the log of the
// current and the previous instance only: older restarts are listed as
// expired, and a deleted pod is not listed at all, since nothing says it
// existed. A Task whose pods are not owned through its Job (a worker pool
// Task, whose pod serves other Tasks too) has no attempts.
//
// # What a read can and cannot tell
//
// Reads ask for timestamps, which give each line its LogTime and let a
// follow that broke resume where it stopped: the lines read in the last
// second must be found again, else a read_interrupted gap says lines may
// be missing. A first read cannot tell whether rotation removed the start
// of the log: the kubelet serves only the current log file. It is detected
// only against an origin the caller already knew (ReadOptions.KnownOrigin).
//
// # RBAC
//
// In each allowlisted namespace: get and list tasks.kelos.dev, get
// jobs.batch, get and list pods, get pods/log.
package kube
