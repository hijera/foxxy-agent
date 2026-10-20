#!/bin/sh
# Runs as root after the package is unpacked, on install and on upgrade.
# FoxxyCode keeps every piece of state under the invoking user's $FOXXYCODE_HOME, so
# there is nothing to create system-wide here. The message points the user at
# the one file they do have to write themselves, and at the systemd user unit
# the package installed and deliberately did not enable: which accounts run a
# server is for each user to decide, with `foxxycode serve install`.
set -e

# Debian passes "configure" and, on an upgrade, the version it replaces; rpm
# passes the number of installed copies, 1 on a first install.
case "${1:-}" in
    1) event=install ;;
    configure) if [ -n "${2:-}" ]; then event=upgrade; else event=install; fi ;;
    [0-9]*) event=upgrade ;;
    *) exit 0 ;;
esac

user_home=""
if [ -n "${SUDO_USER:-}" ]; then
    user_home="$(getent passwd "${SUDO_USER}" | cut -d: -f6)"
fi

if [ "$event" = upgrade ]; then
    # An enabled service keeps running the binary it started with, and an
    # upgrade from a release without the unit is where most users first meet
    # it. Which accounts enabled it is not something root can see from here
    # (each user manager has its own config home), so this goes to everyone.
    cat <<'EOF'

FoxxyCode is upgraded. The systemd user unit /usr/lib/systemd/user/foxxycode.service
is installed and not enabled by the package. As the user the service is for,
without sudo:

    foxxycode serve install   # enable and start it, or restart it on the new binary

EOF
    exit 0
fi

if [ -z "$user_home" ] || [ ! -f "$user_home/.foxxycode/config.yaml" ]; then
    cat <<'EOF'

FoxxyCode is installed. Create your configuration:

    mkdir -p ~/.foxxycode
    cp /usr/share/doc/foxxycode/config.example.yaml ~/.foxxycode/config.yaml

Then set a provider key in it and start a surface:

    foxxycode               # interactive console
    foxxycode serve         # every subsystem config.yaml enables (web UI on by default)
EOF
else
    cat <<'EOF'

FoxxyCode is installed, and ~/.foxxycode/config.yaml is already there.
EOF
fi

cat <<'EOF'

The systemd user unit /usr/lib/systemd/user/foxxycode.service is installed but
NOT enabled. To run foxxycode serve as a service for your account (it works in
~/FoxxyCode and comes back after a crash), run as that user, without sudo:

    foxxycode serve install     # enable and start foxxycode.service
    foxxycode serve uninstall   # stop and disable it again

Manual: man foxxycode   Service guide: https://github.com/hijera/foxxy-agent/blob/main/docs/operate/serve.md

EOF

exit 0
