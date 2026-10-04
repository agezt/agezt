# W2.3 exit evidence

The daemon constructs `app/tools.NewInvoker` separately for primary and tenant
kernels. The same service supplies direct invocation and the five-phase port
used by root and delegated loops. The platform mechanism owns lookup/schema,
policy audit, invocation announcement, safe execution and terminal publication;
callers retain batch admission, memoization, taint preparation, scheduling,
observation formatting, hooks and model result order.

## Executable coverage

All runtime fixtures below use temporary stores and mock providers or runners.
The side-path and dynamic-tool fixtures explicitly inject the app constructor.
Their assertions inspect durable journal events and actual backend entry.

| Entry or guarantee | Permanent evidence |
|---|---|
| Primary/tenant daemon composition | `cmd/agezt/main_toolinvoker_test.go`, `TestOpenAppKernelBindsPrimaryAndTenant` |
| Per-kernel direct binding and policy isolation | `kernel/runtime/tool_invoker_test.go` |
| Root/delegated use of all five bound phases, child identity | `kernel/runtime/loop_phase_port_test.go` |
| Workflow tool, HTTP, pipeline and canvas calls; allow, deny and failures; unique retry IDs | `kernel/runtime/workflow_tool_audit_test.go` |
| Active forge/MCP direct lookup and deny-list enforcement | `TestRunTool_DynamicLookup` in that file |
| Offered and executed forge/MCP tools through the loop | `kernel/runtime/scripttool_test.go`, `kernel/runtime/mcptool_test.go` |
| Council grounding, approvals and failed/refused search handling | `kernel/runtime/council_grounding_audit_test.go` |
| Conductor/workflow/canvas code adapters, policy, approvals, retries and unavailable audit | `kernel/runtime/code_execution_audit_test.go` |
| Artifact-backed audit, caller bytes and full policy details | `kernel/runtime/tool_offload_audit_test.go`, `kernel/runtime/tool_policy_audit_test.go` |
| Batch admission before effects, memo policy order and terminal settlement | `kernel/agent/admission_phase_test.go`, `kernel/agent/tool_terminal_audit_test.go` and sibling internal tests |
| Log/stats join on correlation plus call ID; skipped calls have no execution latency | `kernel/controlplane/tool_audit_identity_test.go` |
| Explicit phase selection, mandatory audit errors and legacy forwarding | `kernel/platform/toolpipeline/phases_test.go`, `kernel/toolexec/phases_test.go` |

The exit run repeats app-bound side-path, dynamic-tool and root/delegated tests
with `-count=20`. Source package suites, full repository tests, build, vet,
static analysis and architecture gates are required before delivery. Mutation
checks for each guarantee are recorded in roadmap slices W2.3a–w; exit checks
also challenge dynamic lookup and per-kernel loop binding independently.

## Source boundary review

Production runtime callers enter `Kernel.RunTool` or invocation-local adapters;
`kernel/app/tools` delegates to the canonical platform service. The loop consumes
the bound phase port. Backend `Tool.Invoke` is called by the shared panic
firewall; tool implementations and plugin transport methods remain backends.

The plugin SDK can execute configured `HostTools` callbacks independently.
The daemon composition does not configure `HostTools`; this is not an active
daemon side path. This exit does not certify arbitrary external SDK consumers,
real remote providers, physical sandbox execution or browser sessions.

Standalone `runtime.Open`, public legacy invoker functions and standalone
`agent.Run` retain canonical defaults. A custom factory implementing only the
one-shot invoker keeps its historical loop behavior through one canonical
host-bound compatibility phase service per kernel. No upward implementation
import or architecture allowlist expansion is required.

W2.3's original workflow, Council, Conductor and direct-denial/dynamic-lookup
findings are closed. File Manager and file snapshot rollback still write in the
WebUI layer and are the next independent operation-pipeline slice (NEXT §4.4).
