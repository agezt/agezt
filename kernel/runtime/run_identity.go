// SPDX-License-Identifier: MIT

package runtime

// The host's RUN IDENTITY: the correlation id a run carries and the bus
// subject its events travel on.
//
// These two doors are the HOST's vocabulary, not the run lifecycle's: the
// adapters (the REST and the OpenAI surfaces) subscribe to a run's events by
// subject, and the run engine and the delegation engine ask the host for the
// id of a run they are about to start. The module's manager mints the same
// ids — it lives in the same module and holds the same registry — but the
// host keeps the door its callers above hold.
//
// W3.76aa moved the LIFECYCLE OPERATIONS (the halt latch, the drain, the
// cancel and the queries over the live-run registry) into the module's port
// (api.LifecycleService); the identity stayed with the host.

// NewCorrelation mints a fresh correlation ID suitable for a run.
func (k *Kernel) NewCorrelation() string { return k.lifecycle.NewCorrelation() }

// SubjectForRun returns the bus subject pattern that matches every event
// emitted by the run identified by corr.
func (k *Kernel) SubjectForRun(corr string) string { return k.lifecycle.SubjectForRun(corr) }
