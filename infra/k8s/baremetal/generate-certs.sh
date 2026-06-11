#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CERT_DIR="$SCRIPT_DIR/certs"
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

echo "Generating Root CA..."
openssl genrsa -out rootCA.key 4096
openssl req -x509 -new -nodes -key rootCA.key -sha256 -days 3650 -out rootCA.pem \
  -subj "/C=TW/O=SkyLab/CN=SkyLab Local Root CA"

echo "Generating Domain Private Key..."
openssl genrsa -out tls.key 2048

echo "Creating CSR config..."
cat > csr.conf <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn

[dn]
C = TW
O = SkyLab
CN = staging.sky-lab.uk

[req_ext]
subjectAltName = @alt_names

[alt_names]
DNS.1 = staging.sky-lab.uk
DNS.2 = grafana-staging.sky-lab.uk
DNS.3 = stage.sky-lab.uk
DNS.4 = grafana-stage.sky-lab.uk
EOF

echo "Generating CSR..."
openssl req -new -key tls.key -out tls.csr -config csr.conf

echo "Creating extensions config..."
cat > v3.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = staging.sky-lab.uk
DNS.2 = grafana-staging.sky-lab.uk
DNS.3 = stage.sky-lab.uk
DNS.4 = grafana-stage.sky-lab.uk
EOF

echo "Signing Certificate with Local Root CA..."
openssl x509 -req -in tls.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial \
  -out tls.crt -days 365 -sha256 -extfile v3.ext

echo "Certificate generation completed successfully!"
