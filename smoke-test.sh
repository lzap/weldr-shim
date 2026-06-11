#!/bin/bash
set -e

# Enable fast manifest-only mode
export MANIFEST_ONLY=1

echo "==> Building weldr-shim"
go build -o /tmp/weldr-shim .

# Check if service is already running
if composer-cli status show >/dev/null 2>&1; then
    echo "==> Service already running, using existing instance"
    SKIP_CLEANUP=1
else
    # Start service in background
    echo "==> Starting weldr-shim"
    sudo -E /tmp/weldr-shim &
    PID=$!
    trap "sudo kill $PID 2>/dev/null || true" EXIT
    SKIP_CLEANUP=0

    # Wait for service to be ready
    echo "Waiting for service to start..."
    for i in {1..30}; do
        if composer-cli status show >/dev/null 2>&1; then
            echo "Service ready"
            break
        fi
        sleep 1
    done
fi

echo "==> Testing status"
composer-cli status show

echo "==> Creating test blueprint"
cat > /tmp/test-bp.toml <<EOF
name = "test-bp"
description = "Test blueprint for smoke test"
version = "0.1.0"

[[packages]]
name = "bash"
version = "*"

[[packages]]
name = "curl"
version = "*"
EOF

echo "==> Pushing blueprint"
composer-cli blueprints push /tmp/test-bp.toml

echo "==> Listing blueprints"
BLUEPRINTS=$(composer-cli blueprints list)
if ! echo "$BLUEPRINTS" | grep -q "test-bp"; then
    echo "ERROR: Blueprint not found in list"
    exit 1
fi
echo "✓ Blueprint listed"

echo "==> Getting blueprint info"
BP_INFO=$(composer-cli blueprints show test-bp)
if ! echo "$BP_INFO" | grep -q "test-bp"; then
    echo "ERROR: Blueprint info missing blueprint name"
    exit 1
fi
if ! echo "$BP_INFO" | grep -q "bash"; then
    echo "ERROR: Blueprint info missing packages"
    exit 1
fi
echo "✓ Blueprint info complete"

echo "==> Starting compose"
COMPOSE_OUTPUT=$(composer-cli compose start test-bp qcow2)
COMPOSE_ID=$(echo "$COMPOSE_OUTPUT" | awk '{print $2}')
echo "Compose ID: $COMPOSE_ID"

if [ -z "$COMPOSE_ID" ]; then
    echo "ERROR: Failed to get compose ID"
    exit 1
fi

echo "==> Waiting for compose to finish (timeout: 1h, poll: 2s)"
if ! composer-cli compose wait "$COMPOSE_ID" --timeout 1h --poll 2s; then
    echo "ERROR: Compose failed or timed out"
    composer-cli compose info "$COMPOSE_ID"
    exit 1
fi
echo "✓ Compose finished successfully"

echo "==> Getting compose info"
composer-cli compose info "$COMPOSE_ID"

echo "==> Listing compose types"
TYPES=$(composer-cli compose types)
if ! echo "$TYPES" | grep -q "qcow2"; then
    echo "ERROR: qcow2 not found in compose types"
    exit 1
fi
echo "✓ Compose types listed"

echo "==> Listing distros"
DISTROS=$(composer-cli distros list)
if ! echo "$DISTROS" | grep -q "fedora-43"; then
    echo "ERROR: fedora-43 not found in distros list"
    exit 1
fi
echo "✓ Distros listed"

echo "==> Checking compose list"
COMPOSE_LIST=$(composer-cli compose list)
if ! echo "$COMPOSE_LIST" | grep -q "$COMPOSE_ID"; then
    echo "ERROR: Compose not found in list"
    exit 1
fi
echo "✓ Compose in list"

echo "==> Checking compose status"
STATUS_ALL=$(composer-cli compose status)
if ! echo "$STATUS_ALL" | grep -q "$COMPOSE_ID"; then
    echo "ERROR: Compose not found in status list"
    exit 1
fi
echo "✓ Compose in status list"

echo "==> Downloading compose image"
if ! composer-cli compose image "$COMPOSE_ID" 2>/dev/null; then
    echo "WARNING: Image download failed (expected in manifest-only mode)"
else
    echo "✓ Image download works"
fi

echo "==> Testing compose cancel"
CANCEL_OUTPUT=$(composer-cli compose start test-bp qcow2)
CANCEL_ID=$(echo "$CANCEL_OUTPUT" | awk '{print $2}')
if [ -n "$CANCEL_ID" ]; then
    composer-cli compose cancel "$CANCEL_ID"
    echo "✓ Compose cancel works"
    # Clean up the canceled compose
    composer-cli compose delete "$CANCEL_ID" 2>/dev/null || true
else
    echo "WARNING: Could not test cancel (no compose ID)"
fi

echo "==> Deleting compose"
composer-cli compose delete "$COMPOSE_ID"

# Verify it's deleted
if composer-cli compose status | grep -q "$COMPOSE_ID"; then
    echo "ERROR: Compose still exists after delete"
    exit 1
fi
echo "✓ Compose deleted"

echo "==> Deleting blueprint"
composer-cli blueprints delete test-bp

# Verify it's deleted
if composer-cli blueprints list | grep -q "test-bp"; then
    echo "ERROR: Blueprint still exists after delete"
    exit 1
fi
echo "✓ Blueprint deleted"

echo ""
echo "==> ✓ All smoke tests passed!"
