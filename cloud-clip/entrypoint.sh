#!/bin/sh
CONFIG_FILE='/app/server-node/config.json'

if [ ! -f $CONFIG_FILE ]; then
echo "#####Generating configuration file#####"
cat>"${CONFIG_FILE}"<<EOF
{
    "server": {
        "host": [
            "${LISTEN_IP:-0.0.0.0}"
        ],
        "port": ${LISTEN_PORT:-9501},
        "uds": "/var/run/cloud-clipboard.sock",
        "prefix": "${PREFIX}",
        "history": ${MESSAGE_NUM:-10},
        "auth": ${AUTH_PASSWORD:-false},
        "historyFile": "/app/server-node/data/history.json",
        "storageDir": "/app/server-node/data/",
        "ssl": {
            "enabled": ${SSL_ENABLED:-false},
            "key": ${SSL_KEY_PATH},
            "cert": ${SSL_CERT_PATH}
        }
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
