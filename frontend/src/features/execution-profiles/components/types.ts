// types.ts — execution-profiles type definitions (extracted from ExecutionProfiles.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

export interface ExecutionProfile {
  id?: string;
  name?: string;
  summary?: string;
  status?: string;
  routed?: boolean;
  requested_isolation?: string;
  effective_isolation?: string;
  degraded?: boolean;
  degrade_reason?: string;
  tools?: string[];
  backends?: string[];
  filesystem?: string;
  network?: string;
  environment?: string;
  secrets?: string;
  secret_policy?: {
    mode?: string;
    scope?: string;
    values_forwarded?: boolean;
    metadata_forwarded?: boolean;
    valid?: boolean;
    detail?: string;
  };
  limits?: string[];
  browser_access?: string;
  cleanup?: string;
  policy_capability?: string;
  notes?: string[];
}
export interface ExecutionProfileInventory {
  host_os?: string;
  host_arch?: string;
  profiles?: ExecutionProfile[];
  count?: number;
  routed_count?: number;
  supported_count?: number;
  degraded_count?: number;
}
export interface ExecutionProfileCheck {
  id?: string;
  profile_id?: string;
  status?: "ok" | "warning" | "fail" | string;
  title?: string;
  detail?: string;
  next?: string;
  routed?: boolean;
  degraded?: boolean;
  backend_available?: boolean;
  backend?: string;
}
export interface ExecutionProfileHealthReport {
  host_os?: string;
  host_arch?: string;
  checks?: ExecutionProfileCheck[];
  count?: number;
  ok_count?: number;
  warning_count?: number;
  fail_count?: number;
  routable_run_profiles?: string[];
}
export interface ConfigValueEntry {
  env?: string;
  value?: string;
  set?: boolean;
  env_pinned?: boolean;
}
export interface ExecutionProfilePolicyValues {
  allow: string;
  deny: string;
  allowPinned: boolean;
  denyPinned: boolean;
}
export interface ExecutionProfileBackendValues {
  sshEnabled: boolean;
  sshTarget: string;
  sshWorkDir: string;
  sshIdentity: string;
  sshPort: string;
  sshStrictHostKey: string;
  k8sEnabled: boolean;
  k8sContext: string;
  k8sNamespace: string;
  k8sPod: string;
  k8sContainer: string;
  k8sWorkDir: string;
  modalEnabled: boolean;
  modalRef: string;
  modalImage: string;
  modalEnvironment: string;
  modalAddPython: string;
  modalWorkDir: string;
  daytonaEnabled: boolean;
  daytonaSandbox: string;
  daytonaWorkDir: string;
  dockerEnabled: boolean;
  dockerRuntime: string;
  dockerImage: string;
  dockerNetwork: string;
  envLocal: string;
  envWarden: string;
  envDocker: string;
  secretEnvLocal: string;
  secretEnvWarden: string;
  secretEnvDocker: string;
  secretFilesLocal: string;
  secretFilesWarden: string;
  secretFilesDocker: string;
  remoteSecretPolicy: string;
  remoteEventMirror: string;
  remoteArtifactBytes: string;
  pinned: Record<string, boolean>;
}
