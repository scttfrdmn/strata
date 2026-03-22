# Package Management in Strata

Strata supports three complementary paths for building compute environments, each with
different trade-offs between reproducibility, auditability, and iteration speed.

---

## The Three-Path Model

| Path | How it works | Artifact type | Audit depth | Best for |
|------|-------------|---------------|-------------|----------|
| **A** | Recipe → `strata build` → squashfs in registry | Signed squashfs layer | Full: SHA256 + Rekor entry + content manifest | Production workloads, published results |
| **B** | Interactive session on persistent EBS upper → `strata freeze-layer` | Signed squashfs layer (same format as Path A) | Same as A after freeze; unattested before | Exploratory work that needs to become reproducible |
| **C** | `packages:` in profile → pinned at `strata freeze` → installed at boot | No squashfs; agent installs at boot | Partial: versions pinned, not content-addressed | User-level packages on top of base layers |

All three paths share the same OverlayFS environment model. Paths A and B produce identical
squashfs artifacts — the difference is how they were created.

---

## Path C: The `packages:` Section

For user-level package installation that does not require a squashfs layer, declare
`packages:` in the profile. The Strata agent installs these at boot using the appropriate
package manager.

### Profile YAML

```yaml
name: ml-workstation
base:
  os: al2023

software:
  - python@3.12
  - gcc@14.2

packages:
  - manager: pip
    packages:
      - name: numpy
        version: "1.26"
      - name: torch
        version: "2.2"
      - name: matplotlib

  - manager: cran
    packages:
      - name: ggplot2
      - name: dplyr
        version: "1.1"

  - manager: conda
    env: ml-base
    packages:
      - name: scipy
        version: "1.11"
```

### How `strata freeze` pins versions

Running `strata freeze ml-workstation.yaml` does two things:

1. Resolves the `software:` list to exact layer SHA256s (as always).
2. Resolves the `packages:` list to exact pinned versions by querying each package manager's
   index. The result is stored in `packages:` blocks in the lockfile.

**Lockfile fragment (after freeze):**

```yaml
packages:
  - manager: pip
    packages:
      - name: numpy
        version: 1.26.4
        sha256: aabbcc...
      - name: torch
        version: 2.2.1
      - name: matplotlib
        version: 3.9.0
  - manager: cran
    packages:
      - name: ggplot2
        version: 3.5.1
      - name: dplyr
        version: 1.1.4
  - manager: conda
    env: ml-base
    packages:
      - name: scipy
        version: 1.11.3
```

### What the agent does at boot

When the agent assembles the environment from a lockfile with a `packages:` section:

1. OverlayFS layers are mounted (the usual path).
2. For each `packages:` block in the lockfile, the agent runs:
   - pip: `pip install <name>==<version>`
   - conda: `conda install -n <env> <name>=<version>`
   - cran: `Rscript -e "install.packages('<name>')"`
3. The `strata:packages-status` EC2 tag is updated: `installing` → `ready`.

### Reproducibility caveats

Packages are pinned by **version string**, not by content hash (except pip wheels which
include a SHA256 when available). This means:

- The same version string could theoretically correspond to different bits if a package
  maintainer republishes under the same version number (rare but possible).
- For full content addressing, use Path A (`strata build` with a recipe).
- For lab work where version pinning is sufficient, Path C is much faster to iterate.

---

## Path B: Mutable Layers

The mutable layer workflow lets you build interactively and then commit the result into
a proper signed squashfs layer — identical in format and trust to a Path A layer.

### Declaring a mutable layer in the profile

```yaml
name: torch-exploration
base:
  os: al2023

software:
  - python@3.12
  - gcc@14.2

mutable_layer:
  name: torch-ml
  version: 0.1.0
  size_gb: 50
  volume_type: gp3
  abi: linux-gnu-2.34
```

### Step-by-step workflow

1. **Resolve and launch**: `strata resolve torch-exploration.yaml` → lockfile with
   `mutable_layer:` set. Launch the instance.

2. **Install interactively**: The agent mounts an EBS volume as the OverlayFS upper.
   Everything you install (`pip install`, `conda install`, `make install`) goes into the
   persistent EBS volume.

3. **Freeze the upper into a layer**:
   ```sh
   strata freeze-layer \
     --upper /strata/upper \
     --name torch-ml \
     --version 0.1.0 \
     --abi linux-gnu-2.34 \
     --arch x86_64 \
     --registry s3://my-strata-bucket \
     --key awskms:///alias/strata-signing-key \
     --provides torch=2.2.1,python=3.12.13
   ```

4. **Use in profiles**: Once in the registry, reference it like any other layer:
   ```yaml
   software:
     - python@3.12
     - torch-ml@0.1.0
   ```

### "Dirty" lockfile semantics

A lockfile with `mutable_layer:` set is considered **dirty** and cannot be published
(via `strata publish`) until `strata freeze-layer` has been called and the layer is in
the registry. This prevents accidental publication of unattested environments.

---

## `strata freeze-layer` Reference

Converts an upper directory into a signed squashfs layer and pushes it to a registry.

```
strata freeze-layer [flags]

Flags:
  --upper      string   Path to the upper directory to freeze (required)
  --name       string   Layer name, e.g. torch-ml (required)
  --version    string   Layer version, e.g. 0.1.0 (required)
  --abi        string   C runtime ABI (default: linux-gnu-2.34)
  --arch       string   Target architecture: x86_64 or arm64 (default: x86_64)
  --registry   string   Registry URL: s3://... or file://...
  --key        string   Cosign key or KMS URI (default: awskms:///alias/strata-signing-key)
  --provides   string   Comma-separated capability=version pairs (e.g. torch=2.2.0,python=3.12.13)
  --requires   string   Comma-separated requirement strings (e.g. glibc@>=2.34,python@>=3.12)
  --dry-run            Print manifest summary without creating squashfs or requiring a registry
```

### Examples

```sh
# Preview without building (no squashfs, no registry needed):
strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
  --abi linux-gnu-2.34 --arch x86_64 --dry-run

# Freeze to a local registry:
strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
  --registry file:///var/strata-local \
  --key ~/strata.key

# Freeze to S3 using KMS:
strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
  --abi linux-gnu-2.34 --arch x86_64 \
  --registry s3://my-strata-bucket \
  --key awskms:///alias/strata-signing-key \
  --provides torch=2.2.1,python=3.12.13 \
  --requires glibc@>=2.34,python@>=3.12
```

---

## `strata snapshot-ami`

Creates an EC2 AMI from the current running instance. This is the **Stage 1 alternative**
to the persistent EBS upper: after installing packages interactively, snapshot the entire
instance state as an AMI.

The AMI can then be used as the `base:` in future profiles, or as `--ami` in `strata build`.
Unlike the EBS upper approach, an AMI snapshot captures the entire instance state (OS +
all installs) without needing to track a separate volume.

### When to use AMI snapshots vs. freeze-layer

| | AMI snapshot | freeze-layer |
|---|---|---|
| Captures | Entire instance (OS + installs) | OverlayFS upper only |
| Use as | Base for future builds/resolves | Layer in the Strata registry |
| Reproducibility | Not content-addressed | Signed squashfs with SHA256 + Rekor |
| Best for | Quick iteration without the registry | Layers you want to share or publish |

### Command reference

```
strata snapshot-ami [flags]

Flags:
  --instance-id    string     EC2 instance ID (default: current instance via IMDS)
  --name           string     AMI name (default: strata-snapshot-<timestamp>)
  --description    string     AMI description
  --no-reboot                 Create without rebooting (may produce inconsistent filesystem)
  --wait                      Wait until AMI reaches 'available' state
  --poll-interval  duration   Polling interval when --wait is set (default: 30s)
  --region         string     AWS region (default: us-east-1)
  --output         string     Output format: text or json (default: text)
```

### Example workflow

```sh
# On an EC2 instance after installing packages interactively:
strata snapshot-ami --wait

# Output:
# snapshot: ami-0123456789abcdef0
#   name:   strata-snapshot-20260321-142533
#   state:  available
#   region: us-east-1
#   hint:   use as base in a profile: base: {os: ami-0123456789abcdef0}
#   hint:   or as --ami in: strata build --ec2 --ami ami-0123456789abcdef0
```

---

## Local Registry (`file://`)

The `file://` scheme allows running a Strata registry entirely on the local filesystem.
This is useful for:

- **Development**: build and test layers without an S3 bucket.
- **Air-gapped HPC clusters**: copy the registry directory to a shared filesystem.
- **CI/CD pipelines**: run a local registry for testing without AWS credentials.

### Usage

```sh
# Set the registry URL via environment variable:
export STRATA_REGISTRY_URL=file:///var/strata-local

# Or use --registry on individual commands:
strata build recipes/core/python/3.12.13 --registry file:///var/strata-local --dry-run

# freeze-layer to local registry:
strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
  --registry file:///var/strata-local
```

### Directory layout

The local registry uses the same layout as S3:

```
/var/strata-local/
  layers/
    linux-gnu-2.34/x86_64/python/3.12.13/
      manifest.yaml
      layer.sqfs
      bundle.json
  formations/
    ml-stack/2024.03/
      manifest.yaml
  probes/
    ami-0abc123/
      capabilities.yaml
  index/
    layers.yaml       ← flat index for fast resolution
  locks/
    <environmentID>.yaml
```

---

## Private and Federated Registries

Declare multiple registries in the profile's `registries:` section. The resolver searches
them in priority order — the first registry that has the layer wins. The public Strata
registry is always appended last as a fallback.

### Profile YAML with multiple registries

```yaml
name: ml-lab-workstation
base:
  os: al2023

registries:
  - url: s3://my-lab-registry
    trust: keyfile://~/.strata/lab.pub
  - url: file:///mnt/shared/strata-cache
    trust: keyfile://~/.strata/lab.pub
  # public registry used automatically as final fallback

software:
  - torch-ml@0.1.0        # from private registry
  - python@3.12           # from public registry (fallback)
  - gcc@14.2              # from public registry (fallback)
```

### Search order

1. `s3://my-lab-registry` — checked first (highest priority)
2. `file:///mnt/shared/strata-cache` — checked second
3. Public Strata registry — always checked last

If the same layer ID appears in multiple registries, the highest-priority result wins.
`ListLayers` merges all registries and deduplicates by layer ID.

---

## Reproducibility Trade-off Table

| Approach | Content-addressed? | Signed? | Rekor entry? | Recommended for |
|----------|-------------------|---------|--------------|-----------------|
| Path A (`strata build`) | Yes (squashfs SHA256) | Yes | Yes | All production use |
| Path B (`strata freeze-layer`) | Yes (squashfs SHA256) | Yes | Yes | Promoted interactive work |
| Path C (`packages:`) | Partial (version only) | No | No | User-level packages, fast iteration |
| AMI snapshot | No (AMI ID only) | No | No | Stage 1 quick iteration |

For a scientific paper's supplementary materials, use Path A or B — these produce a
lockfile with a Rekor entry that independently proves the environment without trusting
Strata's registry or infrastructure.

---

## End-to-End ML Paper Workflow

This example walks through all three stages of a typical ML paper workflow.

### Stage 1: Exploration (fast iteration with AMI snapshot)

```sh
# Resolve a base profile with your framework layers.
strata resolve ml-base.yaml -o ml-base.lock.yaml

# Launch an EC2 instance with the resolved environment.
aws ec2 run-instances ... --user-data <agent-bootstrap>

# On the instance: install experimental packages.
pip install transformers==4.40.0 accelerate==0.30.0
conda install -n base cudnn=8.9

# When happy with the setup, snapshot for reuse.
strata snapshot-ami --wait --name ml-paper-stage1
```

### Stage 2: Reproducible layer (Path B → signed squashfs)

```sh
# Freeze the upper into a signed layer.
strata freeze-layer \
  --upper /strata/upper \
  --name paper-env \
  --version 1.0.0 \
  --registry s3://my-lab-registry \
  --key awskms:///alias/strata-signing-key \
  --provides transformers=4.40.0,accelerate=0.30.0 \
  --requires python@>=3.12,cuda@>=11.8
```

### Stage 3: Paper release (full audit, Path A)

```sh
# Write a proper recipe for full auditability and rebuild cleanly.
# The recipe records every configure flag and produces a squashfs
# that can be independently rebuilt from source.
strata build recipes/application/paper-env/1.0.0 \
  --ec2 --ami ami-0c421724a94bba6d6 \
  --registry s3://my-lab-registry \
  --key awskms:///alias/strata-signing-key

# Resolve the final lockfile with the rebuilt layer.
strata resolve ml-paper.yaml -o ml-paper.lock.yaml

# Publish to Zenodo with a DOI.
strata publish ml-paper.lock.yaml --token $ZENODO_TOKEN
```

The final lockfile contains:
- SHA256 of the profile YAML
- SHA256 + Rekor entry for every layer
- SHA256 of the base AMI snapshot
- A Zenodo DOI for permanent citation

Any reader can independently verify the environment using only the lockfile and standard
`cosign verify` tooling — no Strata tooling required for verification.
