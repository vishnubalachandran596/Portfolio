#!/bin/sh
# Compose mounts the host content directory at /content. Docker Desktop
# on Mac keeps the host uid on that mount, so the unprivileged app user
# cannot read a typical 0644 file until the tree is world-accessible.
# Docker Desktop on Windows often ignores chmod and already presents the
# mount as world-accessible. A failed chmod must not stop startup.
set -e
if [ -d /content ]; then
	chmod -R a+rwX /content 2>/dev/null || true
	if su-exec app sh -c 'test -r /content && test -w /content'; then
		exec su-exec app /usr/local/bin/portfolio
	fi
fi
exec /usr/local/bin/portfolio
