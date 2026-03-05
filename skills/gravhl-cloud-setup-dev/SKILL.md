---
name: cloud-setup-dev
description: Use when making source code changes to cloud-setup, creating branches, pushing changes, or creating merge requests for infrastructure and ArgoCD manifests.
argument-hint: [what to change, e.g. "update nginx replica count to 3"]
---

# Cloud-Setup Development Workflow

Branch/merge-request workflow for the cloud-setup GitOps repo. ArgoCD handles deployment after merge to main.

## Workflow

1. **Start from main:**
```bash
git checkout main && git pull origin main
git checkout -b <branch-name>
```
Use descriptive branch names: `update-nginx-replicas`, `add-redis-config`.

2. **Make changes** to manifests, configs, or docs.

3. **Commit** with a clear message.

4. **Push and create merge request:**
```bash
git push origin <branch-name>
glab mr create --fill --target-branch main
```

5. ArgoCD auto-syncs after merge to main.

## glab Reference

```bash
# Create MR (auto-fills from commits)
glab mr create --fill --target-branch main

# Create MR with explicit title
glab mr create --title "Update nginx replicas" --target-branch main

# List open MRs
glab mr list

# View MR
glab mr view <number>

# Merge an MR
glab mr merge <number>
```

## Rules

- Always branch from main. Never commit directly to main.
- Use `glab` for all GitLab operations.
- ArgoCD handles deployment — no manual kubectl apply.
- Complements `/gravhl-cluster` (cluster ops) and `argocd-skill` (ArgoCD reference).
