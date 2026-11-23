import type { ExecutionProfilesResponseProfilesItemSecretPolicy } from "@/app/api/types.generated";
import type {
  ExecutionProfileCheckResponse,
  ExecutionProfilesResponse,
  ExecutionProfilesResponseProfilesItem,
} from "@/app/api/types.generated";
// types.ts — execution-profiles type definitions (extracted from ExecutionProfiles.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

// ExecutionProfile is an execution profile row as the page reads it.
export type ExecutionProfile = {
  id?: ExecutionProfilesResponseProfilesItem["id"];
  name?: ExecutionProfilesResponseProfilesItem["name"];
  summary?: ExecutionProfilesResponseProfilesItem["summary"];
  status?: ExecutionProfilesResponseProfilesItem["status"];
  routed?: ExecutionProfilesResponseProfilesItem["routed"];
  requested_isolation?: ExecutionProfilesResponseProfilesItem["requested_isolation"];
  effective_isolation?: ExecutionProfilesResponseProfilesItem["effective_isolation"];
  degraded?: ExecutionProfilesResponseProfilesItem["degraded"];
  degrade_reason?: ExecutionProfilesResponseProfilesItem["degrade_reason"];
  tools?: ExecutionProfilesResponseProfilesItem["tools"];
  backends?: ExecutionProfilesResponseProfilesItem["backends"];
  filesystem?: ExecutionProfilesResponseProfilesItem["filesystem"];
  network?: ExecutionProfilesResponseProfilesItem["network"];
  environment?: ExecutionProfilesResponseProfilesItem["environment"];
  secrets?: ExecutionProfilesResponseProfilesItem["secrets"];
  secret_policy?: ExecutionProfilesResponseProfilesItem["secret_policy"];
  limits?: ExecutionProfilesResponseProfilesItem["limits"];
  browser_access?: ExecutionProfilesResponseProfilesItem["browser_access"];
  cleanup?: ExecutionProfilesResponseProfilesItem["cleanup"];
  policy_capability?: ExecutionProfilesResponseProfilesItem["policy_capability"];
  notes?: ExecutionProfilesResponseProfilesItem["notes"];
};

export type ExecutionProfileInventory = {
  host_os?: ExecutionProfilesResponse["host_os"];
  host_arch?: ExecutionProfilesResponse["host_arch"];
  profiles?: ExecutionProfile[] | null;
  count?: ExecutionProfilesResponse["count"];
  routed_count?: ExecutionProfilesResponse["routed_count"];
  supported_count?: ExecutionProfilesResponse["supported_count"];
  degraded_count?: ExecutionProfilesResponse["degraded_count"];
};

export type ExecutionProfileCheck = {
  id?: string;
  profile_id?: string;
  status?: string;
  title?: string;
  detail?: string;
  next?: string;
  routed?: boolean;
  degraded?: boolean;
  backend_available?: boolean;
  backend?: string;
};
export type ExecutionProfileHealthReport = {
  host_os?: ExecutionProfileCheckResponse["host_os"];
  host_arch?: ExecutionProfileCheckResponse["host_arch"];
  checks?: ExecutionProfileCheck[] | null;
  count?: ExecutionProfileCheckResponse["count"];
  ok_count?: ExecutionProfileCheckResponse["ok_count"];
  warning_count?: ExecutionProfileCheckResponse["warning_count"];
  fail_count?: ExecutionProfileCheckResponse["fail_count"];
  routable_run_profiles?: ExecutionProfileCheckResponse["routable_run_profiles"];
};

export type ConfigValueEntry = {
  env?: string;
  value?: string;
  set?: boolean;
  env_pinned?: boolean;
};
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
