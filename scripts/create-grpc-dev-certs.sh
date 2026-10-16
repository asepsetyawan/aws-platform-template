#!/usr/bin/env bash
# Generate a LOCAL DEVELOPMENT CA and server certificate. Does not deploy or overwrite keys.
set -euo pipefail
LAB_ENVIRONMENT=${1:?Usage: create-grpc-dev-certs.sh dev|staging|prod}
case "$LAB_ENVIRONMENT" in dev|staging|prod) ;; *) exit 2 ;; esac
LAB_ROOT=$(cd "$(dirname "$0")/.." && pwd)
LAB_CERT_DIR=${2:-"$LAB_ROOT/.generated/tls/$LAB_ENVIRONMENT"}
if [[ -e "$LAB_CERT_DIR" ]]; then echo "Refusing to overwrite $LAB_CERT_DIR" >&2; exit 1; fi
mkdir -p "$LAB_CERT_DIR"
chmod 700 "$LAB_CERT_DIR"
umask 077
cat > "$LAB_CERT_DIR/ca.cnf" <<'CONFIG'
[req]
distinguished_name = dn
x509_extensions = v3_ca
prompt = no
[dn]
CN = LOCAL DEVELOPMENT ONLY Catalog CA
[v3_ca]
basicConstraints = critical,CA:true
keyUsage = critical,keyCertSign,cRLSign
CONFIG
cat > "$LAB_CERT_DIR/server.cnf" <<CONFIG
[req]
distinguished_name = dn
prompt = no
[dn]
CN = catalog
[v3_server]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:catalog,DNS:catalog.platform-$LAB_ENVIRONMENT.svc.cluster.local,DNS:localhost
CONFIG
openssl req -x509 -newkey rsa:3072 -nodes -days 7 -config "$LAB_CERT_DIR/ca.cnf" \
  -keyout "$LAB_CERT_DIR/ca.key" -out "$LAB_CERT_DIR/ca.crt"
openssl req -new -newkey rsa:3072 -nodes -config "$LAB_CERT_DIR/server.cnf" \
  -keyout "$LAB_CERT_DIR/tls.key" -out "$LAB_CERT_DIR/server.csr"
openssl x509 -req -in "$LAB_CERT_DIR/server.csr" -CA "$LAB_CERT_DIR/ca.crt" -CAkey "$LAB_CERT_DIR/ca.key" \
  -CAcreateserial -days 7 -extfile "$LAB_CERT_DIR/server.cnf" -extensions v3_server -out "$LAB_CERT_DIR/tls.crt"
openssl verify -CAfile "$LAB_CERT_DIR/ca.crt" "$LAB_CERT_DIR/tls.crt"
echo "Development certificates written to $LAB_CERT_DIR; use managed PKI and rotation for production."
