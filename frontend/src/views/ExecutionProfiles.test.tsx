// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import {
  ExecutionProfiles,
  checksByProfileID,
  checkStatusTone,
  executionProfileBackendFromConfigValues,
  executionProfilePolicyFromConfigValues,
  executionProfileRollup,
  profileStatusTone,
} from "@/views/ExecutionProfiles";

const inventory = {
  host_os: "linux",
  host_arch: "amd64",
  count: 2,
  routed_count: 1,
  supported_count: 1,
  degraded_count: 1,
  profiles: [
    {
      id: "local",
      name: "Local host",
      summary: "Direct host execution.",
      status: "supported",
      routed: true,
      requested_isolation: "none",
      effective_isolation: "none",
      tools: ["shell"],
      backends: ["host process"],
      filesystem: "workspace",
      network: "host network",
      secrets: "not forwarded",
      limits: ["timeout"],
      cleanup: "tool-specific",
    },
    {
      id: "docker",
      name: "Docker/OCI",
      summary: "Container-backed execution.",
      status: "planned",
      routed: false,
      degraded: true,
      degrade_reason: "container execution is not routed yet",
      requested_isolation: "container",
      effective_isolation: "not_routed",
      tools: [],
      backends: ["Docker or Podman"],
      filesystem: "bind mount",
      network: "container network",
      secrets: "declared mounts only",
      limits: ["cpu", "memory"],
      cleanup: "container cleanup",
    },
  ],
};

const report = {
  count: 2,
  ok_count: 1,
  warning_count: 1,
  fail_count: 0,
  routable_run_profiles: ["local"],
  checks: [
    {
      id: "local.profile",
      profile_id: "local",
      status: "ok",
      title: "local runtime profile",
      detail: "direct host execution tools are routable",
      routed: true,
    },
    {
      id: "docker.backend",
      profile_id: "docker",
      status: "warning",
      title: "Docker/OCI profile",
      detail: "docker is on PATH, but AGEZT does not route this execution profile yet",
      next: "wire this backend into the tool execution path before advertising it as selectable",
      backend_available: true,
      backend: "docker",
    },
  ],
};

const configValues = {
  fields: [
    { env: "AGEZT_EXEC_PROFILE_ALLOW", value: "local,warden", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_PROFILE_DENY", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SSH", value: "on", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SSH_TARGET", value: "deploy@example.com", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SSH_WORKDIR", value: "/srv/app", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SSH_IDENTITY", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SSH_PORT", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SSH_STRICT_HOST_KEY", value: "accept-new", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S", value: "on", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S_CONTEXT", value: "prod", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S_NAMESPACE", value: "agents", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S_POD", value: "runner-0", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S_CONTAINER", value: "worker", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_K8S_WORKDIR", value: "/workspace", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL", value: "on", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL_REF", value: "app.py::main", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL_IMAGE", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL_ENVIRONMENT", value: "prod", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL_ADD_PYTHON", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_MODAL_WORKDIR", value: "/workspace", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_DAYTONA", value: "on", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_DAYTONA_SANDBOX", value: "sandbox-1", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_DAYTONA_WORKDIR", value: "/workspace", set: true, env_pinned: false },
    { env: "AGEZT_WARDEN_DOCKER", value: "", set: false, env_pinned: false },
    { env: "AGEZT_WARDEN_DOCKER_RUNTIME", value: "docker", set: true, env_pinned: false },
    { env: "AGEZT_WARDEN_DOCKER_IMAGE", value: "python:3.12-slim", set: true, env_pinned: false },
    { env: "AGEZT_WARDEN_DOCKER_NETWORK", value: "none", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_ENV_LOCAL", value: "SAFE_LOCAL", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_ENV_WARDEN", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_ENV_DOCKER", value: "SAFE_DOCKER", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_ENV_LOCAL", value: "GITHUB_TOKEN", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_ENV_WARDEN", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_ENV_DOCKER", value: "OPENAI_API_KEY", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_FILES_LOCAL", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_FILES_WARDEN", value: "", set: false, env_pinned: false },
    { env: "AGEZT_EXEC_SECRET_FILES_DOCKER", value: "OPENAI_API_KEY:openai.key", set: true, env_pinned: false },
    { env: "AGEZT_EXEC_REMOTE_SECRET_POLICY", value: "deny", set: true, env_pinned: false },
    { env: "AGEZT_REMOTE_EVENT_MIRROR", value: "", set: false, env_pinned: false },
    { env: "AGEZT_REMOTE_ARTIFACT_BYTES", value: "", set: false, env_pinned: false },
  ],
};

beforeEach(() => {
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/execution_profiles") return Promise.resolve(inventory);
    if (path === "/api/execution_profile_check") return Promise.resolve(report);
    if (path === "/api/config/values") return Promise.resolve(configValues);
    return Promise.reject(new Error(`unexpected ${path}`));
  });
  postJSON.mockResolvedValue({ applied: "live", saved: true });
});

afterEach(() => {
  cleanup();
  getJSON.mockReset();
  postJSON.mockReset();
});

describe("ExecutionProfiles helpers", () => {
  it("groups checks, tones statuses, and rolls up counts", () => {
    expect(checksByProfileID(report.checks).docker).toHaveLength(1);
    expect(profileStatusTone("supported")).toBe("good");
    expect(profileStatusTone("supported", true)).toBe("warn");
    expect(checkStatusTone("fail")).toBe("bad");
    expect(executionProfilePolicyFromConfigValues(configValues.fields)).toEqual({
      allow: "local,warden",
      deny: "",
      allowPinned: false,
      denyPinned: false,
    });
    expect(executionProfileBackendFromConfigValues(configValues.fields)).toMatchObject({
      sshEnabled: true,
      sshTarget: "deploy@example.com",
      sshWorkDir: "/srv/app",
      sshStrictHostKey: "accept-new",
      k8sEnabled: true,
      k8sContext: "prod",
      k8sNamespace: "agents",
      k8sPod: "runner-0",
      k8sContainer: "worker",
      k8sWorkDir: "/workspace",
      modalEnabled: true,
      modalRef: "app.py::main",
      modalEnvironment: "prod",
      modalWorkDir: "/workspace",
      daytonaEnabled: true,
      daytonaSandbox: "sandbox-1",
      daytonaWorkDir: "/workspace",
      dockerEnabled: false,
      dockerRuntime: "docker",
      dockerImage: "python:3.12-slim",
      dockerNetwork: "none",
      envLocal: "SAFE_LOCAL",
      envDocker: "SAFE_DOCKER",
      secretEnvLocal: "GITHUB_TOKEN",
      secretEnvDocker: "OPENAI_API_KEY",
      secretFilesDocker: "OPENAI_API_KEY:openai.key",
      remoteSecretPolicy: "deny",
      remoteEventMirror: "",
      remoteArtifactBytes: "",
    });
    expect(executionProfileRollup(inventory, report)).toEqual({
      total: 2,
      routed: 1,
      supported: 1,
      degraded: 1,
      selectable: 1,
      warnings: 1,
      failures: 0,
    });
  });
});

describe("ExecutionProfiles view", () => {
  it("renders inventory rows with health check details", async () => {
    render(<ExecutionProfiles />);

    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/execution_profiles"));
    expect(getJSON).toHaveBeenCalledWith("/api/execution_profile_check");

    expect(screen.getByText("Execution Profiles")).toBeTruthy();
    expect(screen.getByText("linux/amd64")).toBeTruthy();
    expect(screen.getByText("Local host")).toBeTruthy();
    expect(screen.getAllByText("Docker/OCI").length).toBeGreaterThan(0);
    expect(screen.getByText("Run Policy")).toBeTruthy();
    expect(screen.getByText("Backend Config")).toBeTruthy();
    expect(screen.getByText("Kubernetes Pod")).toBeTruthy();
    expect(screen.getByText("Modal Shell")).toBeTruthy();
    expect(screen.getAllByText("Daytona Sandbox").length).toBeGreaterThan(0);
    expect(screen.getByText("Env Passthrough")).toBeTruthy();
    expect(screen.getByText("Remote AGEZT")).toBeTruthy();
    expect(screen.getByText("selectable")).toBeTruthy();
    expect(screen.getAllByText("local runtime profile").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/docker is on PATH/).length).toBeGreaterThan(0);
    // Declutter law: each row folds one raw "details" KeyValue — no
    // "not declared" filler cells, empty fields are simply skipped.
    expect(screen.getAllByText("details").length).toBeGreaterThan(0);
    expect(screen.getAllByText("isolation").length).toBeGreaterThan(0);
    expect(screen.queryByText("not declared")).toBeNull();
  });

  it("saves live allow/deny policy through config set", async () => {
    render(<ExecutionProfiles />);

    await waitFor(() => expect(screen.getByText("Run Policy")).toBeTruthy());
    const deny = screen.getByLabelText("Deny");
    fireEvent.change(deny, { target: { value: "docker" } });
    fireEvent.click(screen.getByRole("button", { name: "Save run policy" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_PROFILE_DENY", value: "docker" }),
    );
  });

  it("saves backend and env passthrough config through config set", async () => {
    render(<ExecutionProfiles />);

    await waitFor(() => expect(screen.getByText("Backend Config")).toBeTruthy());
    const target = screen.getByLabelText("SSH Target");
    fireEvent.change(target, { target: { value: "ops@example.com" } });
    const k8sPod = screen.getByLabelText("K8s Pod");
    fireEvent.change(k8sPod, { target: { value: "runner-1" } });
    const modalRef = screen.getByLabelText("Modal Ref");
    fireEvent.change(modalRef, { target: { value: "worker.py::main" } });
    const daytonaSandbox = screen.getByLabelText("Daytona Sandbox");
    fireEvent.change(daytonaSandbox, { target: { value: "sandbox-2" } });
    const dockerEnv = screen.getByLabelText("Docker Env");
    fireEvent.change(dockerEnv, { target: { value: "SAFE_DOCKER, CI" } });
    const dockerFiles = screen.getByLabelText("Docker Secret Files");
    fireEvent.change(dockerFiles, { target: { value: "OPENAI_API_KEY:openai.key, GITHUB_TOKEN" } });
    const remoteSecretPolicy = screen.getByLabelText("Remote Secret Policy");
    fireEvent.change(remoteSecretPolicy, { target: { value: "metadata" } });
    const remoteEventMirror = screen.getByLabelText("Remote Event Mirror");
    fireEvent.change(remoteEventMirror, { target: { value: "redacted" } });
    const remoteArtifactBytes = screen.getByLabelText("Remote Artifact Bytes");
    fireEvent.change(remoteArtifactBytes, { target: { value: "allow" } });
    fireEvent.click(screen.getByRole("button", { name: "Save backend config" }));

    await waitFor(() => {
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_SSH_TARGET", value: "ops@example.com" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_K8S_POD", value: "runner-1" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_MODAL_REF", value: "worker.py::main" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_DAYTONA_SANDBOX", value: "sandbox-2" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_ENV_DOCKER", value: "SAFE_DOCKER, CI" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", {
        name: "AGEZT_EXEC_SECRET_FILES_DOCKER",
        value: "OPENAI_API_KEY:openai.key, GITHUB_TOKEN",
      });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_EXEC_REMOTE_SECRET_POLICY", value: "metadata" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_REMOTE_EVENT_MIRROR", value: "redacted" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_REMOTE_ARTIFACT_BYTES", value: "allow" });
    });
  });
});
