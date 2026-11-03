# Spectro Cloud patches for upstream forklift

The Spectro delta on top of upstream `kubev2v/forklift`, kept as patch files
rather than a permanently diverged branch. Modelled on
`thirdparty-builder/images/mongodb-enterprise`, which applies a patch just
before building and reverts it afterwards.

**Baseline: `v2.12.10`** — patches apply to a pristine checkout of that tag.

## Why

A merge-based fork buries the delta. On the 2.9.2 line, diffing the fork against
the upstream tag produced 20,826 files of mostly vendor churn, which GitHub
truncates at 3,000 — so the 18 files that actually mattered were never rendered.
The same delta as patches is ~360 changed lines across 10 review-sized files,
and it is checkable in two directions: `make check` proves each patch still
applies to a pristine tag, and `make verify` proves the series reproduces the
fork tree byte-for-byte.

It also makes the next upgrade mechanical: check out the new tag, run
`make check`, and only the patches that genuinely conflict need attention.

## Layout

    series      apply order (comments and blanks ignored)
    NNNN-*.patch  one patch per logical change
    Makefile    apply / check / reverse / restore / verify / regenerate

## Usage

    make check                    # dry run: does each patch still apply?
    make apply                    # apply all, in series order
    make reverse                  # revert all, in reverse series order
    make restore                  # discard everything, back to pristine
    make verify BRANCH=<ref>      # prove the patches reproduce <ref> exactly
    make regenerate BRANCH=<ref>  # rebuild patches from a branch

Typical build, mirroring the mongodb-enterprise pattern:

    git checkout v2.12.10
    make -C patches apply
    make build-all-images REGISTRY=... REGISTRY_ORG=... REGISTRY_TAG=...
    make -C patches restore

## The patches

| # | Patch | What it does |
|---|-------|--------------|
| 0001 | raw import | `RequiresConversion()` becomes opt-in via `Provider.Spec.ConvertDisk`. Upstream returns true for vSphere/Ova/HyperV/EC2; we convert only when asked, so raw disk import is the default. |
| 0002 | remove virt-v2v transfer | `ShouldUseV2vForTransfer()` reports false for vSphere, so CDI+VDDK always copies and virt-v2v only converts in place. Drives `CDIDiskCopy=true`, `VirtV2vDiskCopy=false` (skipping `AllocateDisks` and `CopyDisksVirtV2V`) and `ResolveConversionType -> InPlace`. OVA/HyperV keep upstream behaviour. |
| 0003 | in-place disk prep | Truncates disk files still holding partial data from a failed attempt, which otherwise makes `virt-v2v-in-place` fail while probing the format. |
| 0004 | relax confinement | Off OpenShift the conversion pod gets `LIBGUESTFS_BACKEND=direct`, unconfined seccomp/AppArmor and `CAP_SYS_ADMIN`; the target namespace is labelled PSA privileged. OpenShift keeps upstream's profile. |
| 0005 | secure boot, sessions, limits | Sets only `Features.SMM` instead of replacing `Features` wholesale; pairs every vCenter `Login` with a `Logout` (PVM-113); inventory limits 2cpu/1Gi -> 8000m/8Gi and virt-v2v memory 8Gi -> 12Gi; seeds the vSphere OS map only when absent (PEM-8212). |
| 0006 | guard tests | Upstream has no coverage for `RequiresConversion` or `ShouldUseV2vForTransfer`, so nothing catches a refactor that restores upstream semantics. |
| 0007 | fedora virt-v2v + nbdkit | Adds `build-virt-v2v-fedora-image`, bumps the Fedora base 41 -> 43 and virtio-win to 1.9.45-1.el10 (the nbdkit fix the VMO appliance needs). |
| 0008 | virt-v2v tag suffix | The operator bundle passes `VIRT_V2V_IMAGE + PLATFORM_SUFFIX`, so the tag must carry the suffix or the CSV points at an image that does not exist. |
| 0009 | guard test signature | v2.12.7 dropped the `destinationClient` parameter from `ShouldUseV2vForTransfer`; the guard test is updated to the new call shape. |
| 0010 | adapt v2.12.9 test | `plan_transfer_test.go` lands in v2.12.9 asserting the vSphere virt-v2v default this fork inverts. The empty-`excludeDisks` case is reasserted as the CDI path. |
| 0011 | passt as root | libguestfs >= 1.52 gives the appliance its network via passt, which drops to `nobody` before creating its namespaces; the kernel clears capabilities on that setuid, so sandbox setup fails with EPERM and the appliance never gets its network. A wrapper keeps uid 0 in the CentOS Stream 9 virt-v2v images (`Containerfile-upstream`, `-xfs`). Not the Fedora image: passt `0^20261002` (Fedora 43 stable 2026-10-19) creates its userns before dropping privileges (upstream 7bf1595), which fixes the original EPERM, and with `--runas 0` it would map the owner uid 0 and need CAP_SETFCAP, failing with `Couldn't write to /proc/<pid>/uid_map`. |
| 0012 | deep-inspection image | Upstream's `build-deep-inspection-image` points at a `Containerfile-upstream` that does not exist. The operator bundle also neither passed nor declared `DEEP_INSPECTION_IMAGE`, so the CSV envsubst'ed it to empty and warm vSphere plans failed preflight inspection with `deep inspection container image is not set`. Fixes the path, restores the target to the all-images aggregates, and wires the ARG through. Deliberately no RHEL9/XFS variant: the pack ships one deep-inspection image, so `deep_inspection_image_xfs_fqin` stays unset. |
| 0013 | Go 1.26.10 | Pins every ubi9/ubi10 go-toolset builder to 1.26.10 (upstream `main` reaches the same version through floating `go-toolset:9.8`/`10.2` tags; `release-2.12` is still on 1.25.9). The CentOS Stream 9 virt-v2v builders keep their stage, since `cmd/virt-v2v/entrypoint.go` links libvirt through cgo, but copy in the pinned Red Hat toolchain because CentOS's `golang` trails RHEL's. ovirt-populator stayed on ubi8 1.25.9 here because there was no ubi8 1.26.10 yet; see 0014. |
| 0014 | ovirt-populator Go 1.26.10 | Red Hat published ubi8 go-toolset 1.26.10 (`1.26.10-1792672852`) after 0013, so ovirt-populator's three builders move to it and every forklift image now builds with the same Go. The builder stays ubi8; only the toolchain changes. |
| 0015 | CVE dep bumps (PVM-1439) | govulncheck-reachable advisories in `golang.org/x/net`, `x/crypto`, `x/text` and `moby/spdystream`. Root, `cmd/kubectl-mtv` and `cmd/vsphere-copy-offload-populator` move to x/net v0.60.0, x/crypto v0.57.0, x/text v0.42.0 (spdystream v0.5.1 in root). Those x/net and x/crypto releases declare go 1.26, so the directive goes 1.25.0 -> 1.26.10, the patch release the images build with (a bare 1.26.0 makes govulncheck/Bulwark scan the go1.26.0 stdlib and report 34 fixed stdlib advisories), with `godebug default=go1.25` so runtime defaults stay as before (without it `urlstrictcolons` breaks the Vantara client tests). The stdlib advisories are closed by the 1.26.10 toolchain from 0013/0014. The patch carries only `go.mod`/`go.sum`; after `make apply`, run `go mod vendor` in the root module. In the two nested modules, run it and then `git checkout -- vendor/github.com/kubev2v && git clean -fd vendor/github.com/kubev2v`, so their vendored copy of the root module stays upstream's. |

Patches 0002-0004 replace six commits from the 2.9.2 line. v2.12.7 ships the
itinerary/predicate framework the old fork hand-rolled, so the behaviour now
falls out of one change instead of commented-out branches across five files.

0009 and 0010 are per-upgrade adaptations: each time upstream changes the shape
of `ShouldUseV2vForTransfer` or adds a test asserting its own default, the
assertion has to be re-pointed at the inverted behaviour. Expect one such patch
per upgrade.

## Notes

- `make check` replays the series *cumulatively* onto a throwaway worktree of
  `UPSTREAM_TAG` and changes nothing in your own tree, so it reports the
  patches that genuinely conflict in series order. Testing each patch
  independently instead would flag the earlier member of every dependent pair
  (0006/0009, 0007/0008) even when the series is correct. `make verify` is the
  stronger test: it proves the applied result is byte-identical to a branch.
- Verified: applying all 10 to pristine `v2.12.10` yields a tree byte-identical
  to `wip-21210` @ `7a3c0d8b3` — 18 files, +363/-80.
- `v2.12.10` is the first upgrade that needed **no new adaptation patch**. The
  45 files it changed (`.tekton`/`.konflux` SHA and rpm-lock churn,
  `build/release.conf`, `Containerfile-downstream`, and MTV-6912's netplan
  `--root-dir` probe) are disjoint from the 18 files this series touches, so
  every patch — 0002 included — replayed clean. Don't read that as the pattern
  changing: it is still `ShouldUseV2vForTransfer` that decides whether an
  upgrade is interesting.
- Dropped on the move to 2.12.1 because upstream absorbed them: NVMe support
  and its blocking rego, and the dependency downgrades (2.12.1 ships newer
  govmomi/gin/gophercloud than the 2.9.2 line pinned).

## Upgrading to a new upstream tag

1. `git merge <new-tag>` into the fork branch. Releases from v2.12.7 on are real
   ancestry, so this is a plain 3-way merge past the previous tag. Resolve
   nothing by hand unless the intersection below is non-empty. Commit it, and
   note the merge commit — step 4 checks against it.
2. Replay the series onto the new tag to build a fresh `wip-<ver>` chain:
   `git branch -f wip-<ver> <new-tag>` then
   `git cherry-pick <old-tag>..<old-chain-tip>`. This is what actually finds
   conflicts; `make check` only reports them for patches run against a pristine
   tree.
3. Fix conflicts by editing the patch, or re-derive it from intent. 0002 is
   the one to watch: it depends on `ShouldUseV2vForTransfer`, `CDIDiskCopy`,
   `VirtV2vDiskCopy` and `ResolveConversionType` keeping their shape. Upstream
   has hit it in almost every release so far — v2.12.10 is the exception — and
   when it does it also adds a test asserting its own default, so expect a new
   adaptation patch (as 0010).
4. Regenerate and prove it, in this order:
   `make -C patches regenerate BRANCH=wip-<ver> UPSTREAM_TAG=<new-tag>` then
   `make -C patches check UPSTREAM_TAG=<new-tag>` (expect zero `CONFLICT`) and
   `make -C patches verify BRANCH=wip-<ver> UPSTREAM_TAG=<new-tag>` (expect
   `IDENTICAL`). Then diff the two trees you now have:
   `git diff wip-<ver> <merge-commit> -- . ':(exclude)patches'` must be
   **empty** — that is the proof the merged branch and the regenerated series
   encode the same delta.
5. Update `UPSTREAM_TAG` in the Makefile, the `series` baseline comment and the
   baseline noted here.
