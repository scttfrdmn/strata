# Display Stack Design Notes

Captured March 2026. Not blocking any current milestone — recorded for future design work.

---

## The Premise

Traditional HPC (batch jobs, MPI, "submit and wait") is fading. Research computing is
becoming interactive and visual. The cloud instance *is* the workstation — persistent,
GPU-equipped, connected via browser or remote desktop. Strata's tier model handles the
compiler/MPI/math stack cleanly but says nothing about the display stack. This is the gap.

---

## Domains Where Display Is Central to the Science

These are not "nice to have GUI wrappers" — the visualization is the scientific work:

- **Cryo-EM/cryo-ET**: ChimeraX, RELION GUI, cryoSPARC — fitting atomic models into
  density maps is manual, iterative, expert work. Cannot be batched away.
- **Neuroscience imaging**: napari, Neuroglancer — terabyte-scale volumetric data that
  cannot be moved to a laptop for viewing.
- **Geospatial / climate**: Cesium, deck.gl, Pangeo dashboards — time-varying 3D fields
  at planetary scale with interactive exploration.
- **ML/AI**: Weights & Biases, TensorBoard, interactive fine-tuning — highly iterative
  feedback loops where visual inspection drives the next experiment.
- **Digital twins**: real-time sensor fusion + simulation where the visualization is
  also the control interface.
- **Molecular dynamics**: VMD, OVITO — live trajectory visualization as simulations run.

---

## Two Distinct Display Models

### 1. Browser-based visualization (already handled)

JupyterLab, RStudio Server, Streamlit, Plotly Dash, napari-browser, vtk.js. The display
runs in the browser; the Strata layer serves a port. These are unambiguously Tier 2 today
and the trend is accelerating toward this model. No design gap here.

### 2. Native GPU-rendered display (the open question)

ParaView with GPU rendering, ChimeraX, VMD, any tool using OpenGL or Vulkan for
hardware-accelerated visualization. On an EC2 G5/G6 instance with NICE DCV, rendering
happens on the GPU and a compressed stream goes to the client. This requires:

- **NVIDIA GL driver** (`libGL.so.1`, `libEGL.so.1`) — ABI-critical the same way CUDA
  is. A binary linked against `libGL.so` from driver 550 behaves differently from 535.
  Arguably belongs in Tier 0 alongside `cuda`.
- **Mesa** (`libGL.so` from llvmpipe/softpipe) — software GL, always available, useful
  for off-screen rendering and as a safe default on non-GPU instances.
- **Display server** (Xorg, Wayland) or headless framebuffer.
- **Remote display protocol** — NICE DCV, VNC, NoMachine. Infrastructure-level, more
  like the network fabric than a scientific library.

---

## Where the Tier Model May Need to Extend

The current Tier 0 criterion — "compiled code links against this `.so` at build time,
version baked into the binary" — applies equally to the GL stack:

```
Possible Tier 0 additions:
  mesa@24.0        provides: opengl@4.6, vulkan@1.3, egl@1.5   (software; always available)
  nvidia-gl@550    provides: opengl@4.6, vulkan@1.3, egl@1.5   (hardware; requires cuda@12)
```

`paraview@5.12` would declare `requires: opengl@>=3.3` and receive whichever GL
implementation is present. The conflict detector prevents mesa and nvidia-gl from loading
simultaneously — same pattern as MPI implementations.

Display client libraries (`libX11.so`, `libwayland-client.so`) are linked by
applications but are not themselves compiler-level ABI surfaces in the same way. These
might belong in Tier 1.0 as a "display client" capability bundle, or be treated as
`runtime_requires` provided by the host (like `glibc`).

---

## The Remote Desktop Question

NICE DCV (and VNC, NoMachine) is neither a scientific library nor a user application —
it is transport infrastructure, analogous to SSH or a VPN. It may belong in a separate
class: **platform services** that Strata declares compatibility with but does not
provision. The same way `glibc` appears in `runtime_requires` without Strata building it.

A `desktop` formation could bundle the pieces Strata *does* own:

```yaml
# hypothetical desktop-2024a formation
contents:
  - mesa@24.0        # or nvidia-gl@550 for GPU instances
  - xorg-libs@7.7    # libX11, libXext, libXrender — display client libs
  - dcv-client@2024  # NICE DCV session agent (Tier 2)
```

---

## The XR Horizon

VR/AR headsets for molecular visualization (protein folding, drug binding sites),
digital twins viewed spatially, collaborative multi-user scientific environments. These
follow the same GPU + display stack logic but with additional latency and rendering
constraints. Worth noting as a longer-term driver — not actionable yet.

---

## What This Does NOT Change Today

- Build headless (e.g., `--without-x`) for any tool where headless is the correct
  server default. R, Python, most CLI tools.
- Mesa/OSMesa as the off-screen GL path for batch visualization (ParaView server mode,
  automated rendering pipelines).
- Browser-based tools (JupyterLab, RStudio Server) cover the majority of interactive
  research computing needs today.

The gap only materializes when a researcher needs hardware-accelerated interactive 3D
on a cloud GPU instance. That is a real and growing use case, but not the majority
case yet.

---

## Open Questions

1. Does the GL stack belong in Tier 0 (ABI criterion says yes) or is it host
   infrastructure like `glibc` (platform criterion)?
2. Should `mesa` and `nvidia-gl` be modeled as alternatives under the same capability
   (`opengl@4.6`) the way MPI implementations are?
3. Is the display server (Xorg/Wayland) a Strata layer or always host infrastructure?
4. At what point does "interactive cloud workstation" become a first-class Strata
   deployment target alongside "batch HPC job"?
