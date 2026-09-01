---
name: zot-populate
description: Use when Docker Hub rate limits block image pulls, zot cache is missing images, pods are in ImagePullBackOff, or someone asks to populate zot, fill zot cache, or bypass docker hub rate limit.
argument-hint: [image1:tag image2:tag ...] or omit to auto-detect
---

## What This Skill Does

Populates the zot OCI pull-through registry cache on mgmt by pulling images from Docker Hub through this machine's IP (bypassing rate limits on mgmt's IP) and pushing them into zot using OCI-format manifest conversion.

## Background

- Zot runs on the mgmt K3s cluster as a pull-through proxy for `docker.io`
- All workload clusters pull Docker Hub images through zot (NodePort 30500 on mgmt)
- Docker Hub anonymous rate limit: 100 pulls per 6 hours per public IP
- Zot stores images in OCI format only -- it rejects Docker v2 manifest pushes
- When zot's cache is missing an image AND Docker Hub rate limits are hit, pods get stuck in `ImagePullBackOff` with zot returning 404

## Prerequisites

- SSH access to mgmt (`ssh mgmt`)
- `crane` binary (auto-install if missing)
- `curl` and `python3` available locally
- This machine must have a different public IP than mgmt (`136.62.143.210`)

## Steps

### Step 1: Identify Missing Images

If the user provided image arguments, use those. Otherwise, auto-detect from failing pods:

```bash
# Check what's already cached
curl -s http://localhost:30500/v2/_catalog

# Find images failing on workload clusters
ssh mgmt 'SECRET=$(kubectl get secret <cluster>-kubeconfig -o jsonpath="{.data.value}" | base64 -d)
echo "$SECRET" | kubectl --kubeconfig /dev/stdin get events -A | grep "Failed to pull" | grep -oP "docker\.io/\S+\"" | sort -u'
```

### Step 2: Set Up SSH Tunnel to Zot

Forward mgmt's zot NodePort to localhost so we can push from this machine:

```bash
ssh -L 30500:172.29.24.127:30500 -N -f mgmt
```

Verify: `curl -s http://localhost:30500/v2/_catalog`

### Step 3: Install Crane (if needed)

```bash
curl -sL "https://github.com/google/go-containerregistry/releases/latest/download/go-containerregistry_Linux_x86_64.tar.gz" | tar -xz crane
chmod +x crane
```

### Step 4: Transfer Blobs via Crane

For each missing image, run crane copy. It will fail on the manifest push but successfully transfer all blobs:

```bash
./crane copy "docker.io/$IMAGE" "localhost:30500/$IMAGE" --insecure --platform linux/amd64
# Expected: blobs transfer, manifest PUT fails with MANIFEST_INVALID
# This is normal -- blobs are now in zot, we fix the manifest next
```

### Step 5: Convert and Push OCI Manifest

This is the key technique. Docker Hub serves Docker v2 manifests, but zot only accepts OCI manifests. We fetch the manifest, convert the mediaType fields, and PUT it directly.

For each image (`REPO:TAG`):

```bash
# Get Docker Hub anonymous auth token
token=$(curl -s "https://auth.docker.io/token?service=registry.docker.io&scope=repository:$REPO:pull" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")

# Get amd64 digest from manifest list
amd64_digest=$(curl -s "https://registry-1.docker.io/v2/$REPO/manifests/$TAG" \
  -H "Authorization: Bearer $token" \
  -H "Accept: application/vnd.docker.distribution.manifest.list.v2+json" | \
  python3 -c "
import json,sys
ml = json.load(sys.stdin)
manifests = ml.get('manifests', [])
amd = [m['digest'] for m in manifests if m.get('platform',{}).get('architecture')=='amd64']
print(amd[0] if amd else 'SINGLE_ARCH')")

# If single-arch, use tag directly
if [ "$amd64_digest" = "SINGLE_ARCH" ]; then
  amd64_digest="$TAG"
fi

# Fetch the platform-specific manifest
manifest=$(curl -s "https://registry-1.docker.io/v2/$REPO/manifests/$amd64_digest" \
  -H "Authorization: Bearer $token" \
  -H "Accept: application/vnd.docker.distribution.manifest.v2+json")

# Convert Docker v2 mediaTypes to OCI
oci_manifest=$(echo "$manifest" | python3 -c "
import json,sys
m = json.load(sys.stdin)
m['mediaType'] = 'application/vnd.oci.image.manifest.v1+json'
m['config']['mediaType'] = 'application/vnd.oci.image.config.v1+json'
for layer in m.get('layers', []):
    if layer['mediaType'] == 'application/vnd.docker.image.rootfs.diff.tar.gzip':
        layer['mediaType'] = 'application/vnd.oci.image.layer.v1.tar+gzip'
print(json.dumps(m))
")

# Push OCI manifest to zot
code=$(echo "$oci_manifest" | curl -s -o /dev/null -w "%{http_code}" -X PUT \
  "http://localhost:30500/v2/$REPO/manifests/$TAG" \
  -H "Content-Type: application/vnd.oci.image.manifest.v1+json" \
  -d @-)
echo "$REPO:$TAG -> $code"  # Should be 201
```

### Step 6: Verify and Recover Pods

After all images are pushed:

```bash
# Verify images are in zot
curl -s http://localhost:30500/v2/$REPO/tags/list

# Delete ImagePullBackOff pods to force fresh pulls from zot cache
ssh mgmt 'SECRET=$(kubectl get secret <cluster>-kubeconfig -o jsonpath="{.data.value}" | base64 -d)
echo "$SECRET" | kubectl --kubeconfig /dev/stdin delete pods -n <namespace> <pod-name>'
```

### Step 7: Clean Up

```bash
# Kill the SSH tunnel when done
pkill -f "ssh -L 30500"
```

## MediaType Conversion Reference

| Docker v2 | OCI |
|-----------|-----|
| `application/vnd.docker.distribution.manifest.v2+json` | `application/vnd.oci.image.manifest.v1+json` |
| `application/vnd.docker.container.image.v1+json` | `application/vnd.oci.image.config.v1+json` |
| `application/vnd.docker.image.rootfs.diff.tar.gzip` | `application/vnd.oci.image.layer.v1.tar+gzip` |

## Troubleshooting

- **404 from zot**: Check zot logs (`kubectl logs -n zot zot-0`). If logs show `rate limit exceeded [http 429]`, that's Docker Hub rate limiting -- zot translates 429 to 404 for the client.
- **Blobs already exist but manifest fails**: This is expected. Crane transfers blobs but can't push Docker v2 manifests to zot. Use step 5 to convert and push the manifest separately.
- **MANIFEST_INVALID on PUT**: Verify all three mediaType fields were converted. Check with `echo "$oci_manifest" | python3 -m json.tool | grep mediaType`.
- **crane not found**: Install per step 3. It's a single static binary.
- **SSH tunnel port already in use**: `pkill -f "ssh -L 30500"` then retry.

## Important Notes

- **NEVER clear zot's cache** to free disk space without asking the user. Rebuilding the cache from scratch takes hours due to Docker Hub rate limits.
- This skill only ADDS missing images to zot. It never removes or modifies existing cached images.
- The conversion technique works because zot's internal storage is OCI-native. When zot syncs on-demand from Docker Hub, it does the same conversion internally.
- All workload clusters (tgravhl, skylab*, kelwin1) pull through zot. Populating zot once benefits all clusters.
