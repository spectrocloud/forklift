#!/bin/sh
#
# libguestfs >= 1.52 gives the appliance its network via passt, which libguestfs
# starts on the host side, as root, and which sandboxes itself: it builds its own
# mount/UTS/IPC/PID namespaces, chroots into an empty tmpfs and installs a seccomp
# filter. To do that it first drops to "nobody" -- but the kernel clears
# capabilities on that setuid(), so the unshare() that follows cannot create the
# namespaces and the appliance never gets its network:
#
#     You can now start qemu (>= 7.2, with commit 13c6be96618c):
#     Failed to detach isolating namespaces: Operation not permitted
#     Failed to sandbox process, exiting
#     virt-v2v-in-place: error: libguestfs error: passt exited with status 1
#
# Adding CAP_SYS_ADMIN to the container does not help: passt drops the capability
# itself, so the failure is identical for a fully privileged pod. Keeping uid 0
# does help -- --runas 0 makes passt's setuid() a no-op, so the capability is still
# there when the namespaces are created and the sandbox completes. libguestfs
# passes no --runas of its own, so the default (nobody) is what breaks.
#
# The extra argument is only added when we are already root; any other caller is
# passed through untouched, so a pod that runs as a non-root uid (an
# OpenShift-assigned one, say) behaves exactly as it did before. passt still
# sandboxes in every case -- only the uid it ends up as changes.
#
# /usr/bin/passt is the generic passt build, which re-execs "<argv0>.avx2" when the
# CPU supports AVX2, so the original is kept beside this wrapper as passt.orig --
# with a passt.orig.avx2 symlink to passt.avx2 so that dispatch keeps working.
# (Without it passt still runs, but takes its generic code path and logs "Can't run
# AVX2 build, using non-AVX2 version" on every invocation.)
#
case " $* " in
    *" --runas "*) exec /usr/bin/passt.orig "$@" ;;
esac

if [ "$(id -u)" = 0 ]; then
    exec /usr/bin/passt.orig --runas 0 "$@"
fi

exec /usr/bin/passt.orig "$@"
