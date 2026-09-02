# Spectro Cloud patches for upstream forklift

The Spectro delta on top of upstream `kubev2v/forklift`, kept as patch files
rather than a permanently diverged branch. Modelled on
`thirdparty-builder/images/mongodb-enterprise`, which applies a patch just
before building and reverts it afterwards.

**Baseline: `v2.12.1`** — patches apply to a pristine checkout of that tag.

## Why

A merge-based fork makes the delta unreviewable. Merging upstream `v2.12.1`
into `spectro-main` produces a 20,826-file diff that GitHub truncates at 3,000
files, so the 17 files that actually matter are never rendered. The same delta
as patches is 906 lines across 8 files, each one reviewable on its own.

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

    git checkout v2.12.1
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

Patches 0002-0004 replace six commits from the 2.9.2 line. v2.12.1 ships the
itinerary/predicate framework the old fork hand-rolled, so the behaviour now
falls out of one change instead of commented-out branches across five files.

## Notes

- `make check` reports `already-in` for patches already present in the tree.
  A patch whose lines a later patch rewrites (0007, rewritten by 0008) reports
  `CONFLICT` when checked out of order — that is expected, not a defect.
  `make verify` is the authoritative test.
- Verified: applying all 8 to pristine `v2.12.1` yields a tree byte-identical
  to `spectro-main-v2.12.1-upgrade` @ `888c0eccd` — 17 files, +348/-75.
- Dropped on the move to 2.12.1 because upstream absorbed them: NVMe support
  and its blocking rego, and the dependency downgrades (2.12.1 ships newer
  govmomi/gin/gophercloud than the 2.9.2 line pinned).

## Upgrading to a new upstream tag

1. `git checkout <new-tag>`
2. `make -C patches check` — see which patches still apply
3. Fix conflicts by editing the patch, or re-derive it from intent. 0002 is
   the one to watch: it depends on `ShouldUseV2vForTransfer`, `CDIDiskCopy`,
   `VirtV2vDiskCopy` and `ResolveConversionType` keeping their shape.
4. `make -C patches verify BRANCH=<ref>` once a branch exists
5. Update `UPSTREAM_TAG` in the Makefile and the baseline noted here
