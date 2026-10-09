# W2.42e typed conductor roles

The Conductor's role preview, `conductor_roles`, now runs on the shared
dispatcher in `kernel/app/council`, beside the membership operations whose
panel it reads. It shows which model fills each role when a Conductor run does
not supply roles.

Running the Conductor, `conductor_ask`, streams its loop and stays native with
the streaming cognition commands.

`Service.Roles` reads the same panel port as `council_members`. `Operations`
adds one read-only, primary-only, primary-tenancy spec with unknown input
allowed and no Web UI route, as before.

Removed with the move:

- the handler, from `conductor.go`;
- its registration in `registerCognitionCommands`.

## Preserved behavior

- `available_models` lists the default panel's models in the kernel's order. It
  is always an array.
- The thinker, worker and verifier take the models round-robin: the first,
  second and third, wrapping when there are fewer. All three are empty when
  there is no panel.
- `auto_filled` is always true.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness runs the pre-slice handler on one kernel and the registered
operation on another. 90 steps repeated twenty times cover five panels, each
with three steps under primary, wrong and tenant tokens, in normal and
canceled contexts:

- no panel;
- an empty panel;
- one model;
- two models;
- duplicate and blank seats and models, including an HTML-bearing model.

Responses are byte-exact, the journals are equal, and no read journals
anything. Each panel's assignment is asserted. The exceptions are 15 canceled
primary-token steps, which return the admission error.

Permanent tests cover:

- the round-robin assignment for no panel and for one, two and four models;
- the spec and its schemas;
- the native binding test, which now also checks that the roles follow the
  primary kernel's panel after an edit, and the registry flags.

The existing conductor suites pass unchanged through the typed path.

Nine independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
