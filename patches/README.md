# Spectro Cloud patches for upstream forklift

The Spectro delta on top of upstream `kubev2v/forklift`, kept as patch files
rather than a permanently diverged branch. Modelled on
`thirdparty-builder/images/mongodb-enterprise`, which applies a patch just
before building and reverts it afterwards.

**Baseline: `v2.12.9`** — patches apply to a pristine checkout of that tag.

## Why

A merge-based fork makes the delta unreviewable. Merging upstream `v2.12.9`
into `spectro-main` produces a 20,826-file diff that GitHub truncates at 3,000
files, so the 18 files that actually matter are never rendered. The same delta
as patches is ~450 changed lines across 10 patches, each one reviewable on its
own.

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

    git checkout v2.12.9
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

Patches 0002-0004 replace six commits from the 2.9.2 line. v2.12.7 ships the
itinerary/predicate framework the old fork hand-rolled, so the behaviour now
falls out of one change instead of commented-out branches across five files.

0009 and 0010 are per-upgrade adaptations: each time upstream changes the shape
of `ShouldUseV2vForTransfer` or adds a test asserting its own default, the
assertion has to be re-pointed at the inverted behaviour. Expect one such patch
per upgrade.

## Notes

- `make check` tests each patch *independently* against the tree it is run in,
  so it is only meaningful against a pristine `UPSTREAM_TAG` checkout. A patch
  whose lines a later patch rewrites reports `CONFLICT` even when the series is
  correct — currently 0007 (rewritten by 0008) and 0006 (rewritten by 0009).
  That is expected, not a defect. `make verify` is the authoritative test.
- Verified: applying all 10 to pristine `v2.12.9` yields a tree byte-identical
  to `wip-2129` @ `509a0a85d` — 18 files, +363/-80.
- Dropped on the move to 2.12.1 because upstream absorbed them: NVMe support
  and its blocking rego, and the dependency downgrades (2.12.1 ships newer
  govmomi/gin/gophercloud than the 2.9.2 line pinned).

## Upgrading to a new upstream tag

1. `git checkout <new-tag>`
2. Replay the series onto the new tag (`git cherry-pick <old-tag>..<chain-tip>`
   from the previous `wip-*` chain) — this is what actually finds conflicts;
   `make check` only reports them for the patches run against a pristine tree.
3. Fix conflicts by editing the patch, or re-derive it from intent. 0002 is
   the one to watch: it depends on `ShouldUseV2vForTransfer`, `CDIDiskCopy`,
   `VirtV2vDiskCopy` and `ResolveConversionType` keeping their shape. Upstream
   has hit it in every release so far, and each time it also adds a test that
   asserts its own default — expect a new adaptation patch (as 0010).
4. Regenerate and prove it:
   `make -C patches regenerate BRANCH=<chain> UPSTREAM_TAG=<new-tag>` then
   `make -C patches verify BRANCH=<chain> UPSTREAM_TAG=<new-tag>`
5. Update `UPSTREAM_TAG` in the Makefile, the `series` baseline comment and the
   baseline noted here.
6. `make check` against a pristine new-tag checkout should then report zero
   `CONFLICT` for patches no later patch rewrites.
