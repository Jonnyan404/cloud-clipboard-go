#!/bin/sh
CONFIG_FILE='/app/server-node/config.json'
DOMAIN_RECORD_FILE='/app/server-node/data/domain_record.txt'

# --- Determine SSL Configuration ---
KEY=""
CERT=""
# SSL_ENABLED_SOURCE="none" # Possible values: 'none', 'manual', 'mkcert'

# 1. Check for Manual SSL Paths (Highest Priority)
if [ -n "${MANUAL_KEY_PATH}" ] && [ -n "${MANUAL_CERT_PATH}" ]; then
    echo "Manual SSL paths provided: KEY='${MANUAL_KEY_PATH}', CERT='${MANUAL_CERT_PATH}'"
    # Check if the specified manual files exist
    if [ -f "${MANUAL_KEY_PATH}" ] && [ -f "${MANUAL_CERT_PATH}" ]; then
        KEY="${MANUAL_KEY_PATH}"
        CERT="${MANUAL_CERT_PATH}"
        # SSL_ENABLED_SOURCE="manual"
        echo "Using manually specified SSL certificate files."
    else
        # Files not found, print warning and disable SSL
        echo "Warning: Manual SSL paths specified, but files not found at '${MANUAL_KEY_PATH}' or '${MANUAL_CERT_PATH}'. SSL will be disabled." >&2
        # KEY and CERT remain empty, SSL_ENABLED_SOURCE remains 'none'
    fi

# 2. Check for MKCERT_DOMAIN_OR_IP (Second Priority)
elif [ -n "${MKCERT_DOMAIN_OR_IP}" ]; then
    echo "MKCERT_DOMAIN_OR_IP is set ('${MKCERT_DOMAIN_OR_IP}'). Managing certificates via mkcert..."
    # SSL_ENABLED_SOURCE="mkcert"
    # Define default paths for mkcert generated files
    MKCERT_KEY_PATH="/app/server-node/data/key.pem"
    MKCERT_CERT_PATH="/app/server-node/data/cert.pem"
    CURRENT_DOMAIN=${MKCERT_DOMAIN_OR_IP}

    # --- mkcert Certificate Generation/Validation Logic ---
    REGENERATE_CERT=false
    # Check if cert files exist at the expected mkcert location
    if [ ! -f "$MKCERT_KEY_PATH" ] || [ ! -f "$MKCERT_CERT_PATH" ]; then
        echo "mkcert SSL certificates not found. Will generate new ones."
        REGENERATE_CERT=true
    # Check domain record file
    elif [ ! -f "$DOMAIN_RECORD_FILE" ]; then
        echo "Domain record file not found. Will generate new certificates."
        REGENERATE_CERT=true
    else
        # Read previous domain
        PREVIOUS_DOMAIN="$(cat "$DOMAIN_RECORD_FILE")"
        # Compare domains
        if [ "$CURRENT_DOMAIN" != "$PREVIOUS_DOMAIN" ]; then
            echo "Domain/IP changed from '$PREVIOUS_DOMAIN' to '$CURRENT_DOMAIN'. Will generate new certificates."
            REGENERATE_CERT=true
        else
            echo "Domain/IP unchanged. Using existing mkcert certificates."
        fi
    fi
    # Generate certificate if needed
    if [ "$REGENERATE_CERT" = true ]; then
        echo "##### Generating SSL certificate via mkcert #####"
        echo "##### Domain/IP: ${CURRENT_DOMAIN} #####"
        mkcert -key-file "$MKCERT_KEY_PATH" -cert-file "$MKCERT_CERT_PATH" "$CURRENT_DOMAIN"
        if [ $? -ne 0 ]; then
            echo "Error: Failed to generate SSL certificates with mkcert." >&2
            exit 1 # Exit on mkcert failure
        fi
        # Record the domain used for generation
        printf "%s" "$CURRENT_DOMAIN" > "$DOMAIN_RECORD_FILE"
        echo "mkcert certificates generated successfully."
    fi
    # Set KEY and CERT to the mkcert paths
    KEY="$MKCERT_KEY_PATH"
    CERT="$MKCERT_CERT_PATH"

# 3. No SSL Configuration Provided
else
    echo "Neither manual SSL paths nor MKCERT_DOMAIN_OR_IP are set. SSL is disabled."
    # KEY and CERT remain empty, SSL_ENABLED_SOURCE remains 'none'
fi



if [ ! -f $CONFIG_FILE ]; then
echo "#####Generating configuration file#####"
cat>"${CONFIG_FILE}"<<EOF
{
    "server": {
        "host": [
            "${LISTEN_IP:-0.0.0.0}",
            "${LISTEN_IP6}"
        ],
        "port": ${LISTEN_PORT:-9501},
        "uds": "/var/run/cloud-clipboard.sock",
        "prefix": "${PREFIX}",
        "key": "${KEY}",
        "cert": "${CERT}",
        "history": ${MESSAGE_NUM:-10},
        "auth": ${AUTH_PASSWORD:-false},
        "historyFile": "/app/server-node/data/history.json",
        "storageDir": "/app/server-node/data/",
        "roomList": ${ROOM_LIST:-false},
        "roomCleanup": 3600
    },
    "text": {
        "limit": ${TEXT_LIMIT:-4096}
    },
    "file": {
        "expire": ${FILE_EXPIRE:-3600},
        "chunk": 1048576,
        "limit": ${FILE_LIMIT:-104857600}
    }
}
EOF
else
    echo "#####Configuration file already exists#####"
fi

cd /app/server-node && ./cloud-clipboard-go
exec "$@"
