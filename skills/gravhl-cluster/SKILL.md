---
name: gravhl-cluster
description: Use when someone mentions gravhl cluster, k8s pods, kubernetes, kubectl, cluster operations, ArgoCD apps, deployments, services, namespaces, pod logs, crashlooping, or any work on the gravhl infrastructure. Also triggers for /gravhl-cluster with a free-form task.
argument-hint: [task description, e.g. "check why nginx is crashlooping"]
---

# GravHL Cluster Operations

Full-toolkit k8s operations on gravhl clusters. Handles connectivity, kubectl commands, ArgoCD CRD operations, and session logging.

## Cluster Registry

All cluster configs live in `.claude/k8s/clusters.yaml` (relative to the `backend` repo root at `~/workspace/gravhl/backend/`).

**On every invocation**, read the registry:
```bash
cat ~/workspace/gravhl/backend/.claude/k8s/clusters.yaml
```

Then determine which cluster to use:
1. If the user specifies a cluster name (e.g., "on mgmt" or "main cluster"), match it to a key in `clusters`.
2. Otherwise, match the current working directory (`cwd`) against each cluster's `repo` field. Use the cluster whose `repo` is a prefix of `cwd`.
3. If no match, list the available clusters and ask the user which one to use.

## Connection Setup

**Every invocation must start with these steps, in order:**

1. Read the cluster registry and resolve the target cluster (see above).

2. Set the kubeconfig using the cluster's `k8sconfig` value:
```bash
export KUBECONFIG=<cluster.k8sconfig>
```

3. Test connectivity:
```bash
kubectl cluster-info --request-timeout=5s 2>&1
```

4. **If connection fails and `tunnel_access` is true:**
   - Tell the user: "Cannot reach the cluster. Attempting to establish SSH tunnel via `<cluster.tunnel>`..."
   - Check if the tunnel is already up: `lsof -i :<cluster.tunnel_check_port> | grep ssh`
   - If not up, run: `ssh -fN <cluster.tunnel>`
   - If the SSH command fails (e.g., agent not loaded, key not unlocked), tell the user exactly what error occurred and ask them to resolve it (likely needs KeePassXC SSH agent unlock)
   - After SSH succeeds, retry `kubectl cluster-info --request-timeout=5s`
   - If still failing after tunnel is up, report the error and stop

5. Once connected, proceed with the requested task.

## Executing Tasks

### kubectl Operations

- Always use `--kubeconfig <cluster.k8sconfig>` or ensure KUBECONFIG is exported
- Default to `--all-namespaces` when the user doesn't specify a namespace
- No confirmation needed for destructive operations — execute directly
- Common patterns:

```bash
# Pod status across all namespaces
kubectl get pods -A

# Describe a failing pod
kubectl describe pod <name> -n <namespace>

# Tail logs
kubectl logs <pod> -n <namespace> --tail=100 -f

# Events sorted by time
kubectl get events -A --sort-by='.lastTimestamp'

# Resource usage
kubectl top pods -A
kubectl top nodes
```

### ArgoCD Operations

**Prefer kubectl with ArgoCD CRDs over the argocd CLI.** This is the preferred approach for AI-driven operations.

```bash
# List all ArgoCD Applications
kubectl get applications -n argocd

# Get app details
kubectl get application <name> -n argocd -o yaml

# Check sync/health status
kubectl get applications -n argocd -o custom-columns='NAME:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status'

# Force a sync by patching the operation
kubectl patch application <name> -n argocd --type merge -p '{"operation":{"initiatedBy":{"username":"claude"},"sync":{"revision":"HEAD"}}}'

# Check app conditions/errors
kubectl get application <name> -n argocd -o jsonpath='{.status.conditions}'
```

For detailed ArgoCD reference (specs, generators, hooks, troubleshooting), see the `argocd-skill` at `.claude/skills/argocd-skill/SKILL.md`.

### Manifest Changes (GitOps)

When changes require modifying manifests:

1. Identify the correct GitOps repo from the cluster's `repo` field in the registry.
2. Application repos are at: `~/workspace/gravhl/backend/gravhl-*/`
3. **Do not create new Helm releases.** Drive all changes through ArgoCD.
4. Edit manifests in the GitOps repo, then let ArgoCD sync the changes.

## Session Logging

After completing the task, append a log entry to the cluster's repo:

```bash
LOG_DIR=<cluster.repo>/temp/logs/k8s
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/$(date +%Y-%m-%d).log"
```

Each log entry format:
```
--- [TIMESTAMP] ---
Cluster: <cluster.name>
Task: <what was requested>
Actions:
- <command 1>: <brief result>
- <command 2>: <brief result>
Result: <outcome summary>
---
```

Write the log entry using the Write or Edit tool (append to the file if it exists, create if it doesn't).

## Important Rules

- **No new Helm releases.** Use ArgoCD for all deployments.
- **ArgoCD CRDs over argocd CLI.** Use kubectl against Application/AppProject resources.
- **Always verify connectivity first.** Never skip the connection check.
- **SSH tunnel issues = ask the user.** Don't try to debug SSH/KeePassXC — just report the error.
- **Free-form arguments.** If invoked as `/gravhl-cluster <task>`, treat the argument as the task to perform.
- **If no argument provided**, ask the user what they need to do on the cluster.
