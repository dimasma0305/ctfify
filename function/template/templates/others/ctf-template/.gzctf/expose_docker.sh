#!/bin/bash

# Docker API Security Script
# Configures iptables to restrict Docker API access to specific source IPs only

set -e

# Configuration variables
DOCKER_API_PORT=${DOCKER_API_PORT:-2376}  # Docker API port (2376 for TLS, 2375 for non-TLS)
DOCKER_API_HOST=${DOCKER_API_HOST:-"0.0.0.0"}  # Docker API bind address
ALLOWED_IPS=${ALLOWED_IPS:-"127.0.0.1 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16"}  # Space-separated list of allowed IPs/CIDRs
CHAIN_NAME="DOCKER_API_SECURITY"
LOG_PREFIX="[DOCKER-API-BLOCKED]"
DOCKER_DAEMON_PID_FILE="/var/run/docker-api.pid"
DOCKER_DAEMON_LOG_FILE="/var/log/docker-api.log"
TLS_CERT_DIR="/etc/docker/certs"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root"
        exit 1
    fi
}

# Function to backup current iptables rules
backup_iptables() {
    local backup_file="/tmp/iptables-backup-$(date +%Y%m%d-%H%M%S).rules"
    print_status "Backing up current iptables rules to $backup_file"
    iptables-save > "$backup_file"
    echo "$backup_file"
}

# Function to create custom chain
create_chain() {
    print_status "Creating custom chain: $CHAIN_NAME"
    
    # Create the chain if it doesn't exist
    if ! iptables -L "$CHAIN_NAME" >/dev/null 2>&1; then
        iptables -N "$CHAIN_NAME"
        print_status "Created new chain: $CHAIN_NAME"
    else
        print_warning "Chain $CHAIN_NAME already exists, flushing rules"
        iptables -F "$CHAIN_NAME"
    fi
}

# Function to add allowed IP rules
add_allowed_ips() {
    print_status "Adding allowed IP rules to chain: $CHAIN_NAME"
    
    for ip in $ALLOWED_IPS; do
        print_status "Allowing access from: $ip"
        iptables -A "$CHAIN_NAME" -s "$ip" -j ACCEPT
    done
}

# Function to add logging and drop rule
add_drop_rule() {
    print_status "Adding logging and drop rules"
    
    # Log blocked connections (limit to prevent log flooding)
    iptables -A "$CHAIN_NAME" -m limit --limit 5/min --limit-burst 10 -j LOG --log-prefix "$LOG_PREFIX "
    
    # Drop all other connections
    iptables -A "$CHAIN_NAME" -j DROP
}

# Function to apply the chain to Docker API port
apply_chain() {
    print_status "Applying security chain to Docker API port $DOCKER_API_PORT"
    
    # Remove existing rule if it exists
    iptables -D INPUT -p tcp --dport "$DOCKER_API_PORT" -j "$CHAIN_NAME" 2>/dev/null || true
    
    # Add rule to jump to our custom chain for Docker API port
    iptables -I INPUT -p tcp --dport "$DOCKER_API_PORT" -j "$CHAIN_NAME"
}

# Function to display current rules
show_rules() {
    print_status "Current iptables rules for Docker API security:"
    echo
    echo "=== Chain: $CHAIN_NAME ==="
    iptables -L "$CHAIN_NAME" -v -n --line-numbers 2>/dev/null || print_warning "Chain $CHAIN_NAME does not exist"
    echo
    echo "=== INPUT chain rules for port $DOCKER_API_PORT ==="
    iptables -L INPUT -v -n --line-numbers | grep -E "(Chain INPUT|$DOCKER_API_PORT|$CHAIN_NAME)" || print_warning "No rules found for port $DOCKER_API_PORT"
}

# Function to remove all rules
cleanup() {
    print_status "Cleaning up Docker API security rules"
    
    # Remove jump rule from INPUT chain
    iptables -D INPUT -p tcp --dport "$DOCKER_API_PORT" -j "$CHAIN_NAME" 2>/dev/null || true
    
    # Flush and delete custom chain
    if iptables -L "$CHAIN_NAME" >/dev/null 2>&1; then
        iptables -F "$CHAIN_NAME"
        iptables -X "$CHAIN_NAME"
        print_status "Removed chain: $CHAIN_NAME"
    fi
}

# Function to test connectivity
test_connection() {
    local test_ip=${1:-"127.0.0.1"}
    print_status "Testing Docker API connectivity from $test_ip"
    
    if command -v docker >/dev/null 2>&1; then
        if timeout 5 docker -H tcp://$test_ip:$DOCKER_API_PORT version >/dev/null 2>&1; then
            print_status "✓ Connection successful from $test_ip"
        else
            print_warning "✗ Connection failed from $test_ip (this may be expected if IP is not in allowed list)"
        fi
    else
        print_warning "Docker client not available for testing"
    fi
}

# Function to check if Docker daemon is running
is_docker_running() {
    if [[ -f "$DOCKER_DAEMON_PID_FILE" ]]; then
        local pid=$(cat "$DOCKER_DAEMON_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            return 0
        else
            rm -f "$DOCKER_DAEMON_PID_FILE"
            return 1
        fi
    fi
    return 1
}

# Function to generate TLS certificates
generate_tls_certs() {
    if [[ ! -d "$TLS_CERT_DIR" ]]; then
        print_status "Creating TLS certificate directory: $TLS_CERT_DIR"
        mkdir -p "$TLS_CERT_DIR"
    fi

    if [[ ! -f "$TLS_CERT_DIR/server-cert.pem" ]]; then
        print_status "Generating TLS certificates for Docker API"
        
        # Generate CA private key
        openssl genrsa -out "$TLS_CERT_DIR/ca-key.pem" 4096
        
        # Generate CA certificate
        openssl req -new -x509 -days 365 -key "$TLS_CERT_DIR/ca-key.pem" \
            -sha256 -out "$TLS_CERT_DIR/ca.pem" -subj "/C=US/ST=CA/L=SF/O=Docker/CN=CA"
        
        # Generate server private key
        openssl genrsa -out "$TLS_CERT_DIR/server-key.pem" 4096
        
        # Generate server certificate signing request
        openssl req -subj "/C=US/ST=CA/L=SF/O=Docker/CN=docker-daemon" \
            -sha256 -new -key "$TLS_CERT_DIR/server-key.pem" \
            -out "$TLS_CERT_DIR/server.csr"
        
        # Generate server certificate
        echo "subjectAltName = DNS:localhost,IP:127.0.0.1,IP:0.0.0.0" > "$TLS_CERT_DIR/extfile.cnf"
        echo "extendedKeyUsage = serverAuth" >> "$TLS_CERT_DIR/extfile.cnf"
        
        openssl x509 -req -days 365 -sha256 -in "$TLS_CERT_DIR/server.csr" \
            -CA "$TLS_CERT_DIR/ca.pem" -CAkey "$TLS_CERT_DIR/ca-key.pem" \
            -out "$TLS_CERT_DIR/server-cert.pem" -extfile "$TLS_CERT_DIR/extfile.cnf" \
            -CAcreateserial
        
        # Generate client private key
        openssl genrsa -out "$TLS_CERT_DIR/client-key.pem" 4096
        
        # Generate client certificate signing request
        openssl req -subj "/C=US/ST=CA/L=SF/O=Docker/CN=client" \
            -new -key "$TLS_CERT_DIR/client-key.pem" \
            -out "$TLS_CERT_DIR/client.csr"
        
        # Generate client certificate
        echo "extendedKeyUsage = clientAuth" > "$TLS_CERT_DIR/extfile-client.cnf"
        openssl x509 -req -days 365 -sha256 -in "$TLS_CERT_DIR/client.csr" \
            -CA "$TLS_CERT_DIR/ca.pem" -CAkey "$TLS_CERT_DIR/ca-key.pem" \
            -out "$TLS_CERT_DIR/client-cert.pem" -extfile "$TLS_CERT_DIR/extfile-client.cnf" \
            -CAcreateserial
        
        # Set appropriate permissions
        chmod 400 "$TLS_CERT_DIR/ca-key.pem" "$TLS_CERT_DIR/server-key.pem" "$TLS_CERT_DIR/client-key.pem"
        chmod 444 "$TLS_CERT_DIR/ca.pem" "$TLS_CERT_DIR/server-cert.pem" "$TLS_CERT_DIR/client-cert.pem"
        
        # Clean up
        rm -f "$TLS_CERT_DIR/server.csr" "$TLS_CERT_DIR/client.csr" "$TLS_CERT_DIR/extfile.cnf" "$TLS_CERT_DIR/extfile-client.cnf"
        
        print_status "TLS certificates generated successfully"
    else
        print_status "TLS certificates already exist"
    fi
}

# Function to start Docker daemon
start_docker_daemon() {
    if is_docker_running; then
        print_warning "Docker API daemon is already running (PID: $(cat $DOCKER_DAEMON_PID_FILE))"
        return 0
    fi

    print_status "Starting Docker API daemon"
    
    # Check if Docker is installed
    if ! command -v dockerd >/dev/null 2>&1; then
        print_error "Docker daemon (dockerd) not found. Please install Docker first."
        exit 1
    fi

    # Generate TLS certificates if using secure port
    if [[ "$DOCKER_API_PORT" == "2376" ]]; then
        generate_tls_certs
        
        # Start Docker daemon with TLS
        print_status "Starting Docker daemon with TLS on $DOCKER_API_HOST:$DOCKER_API_PORT"
        nohup dockerd \
            --host="tcp://$DOCKER_API_HOST:$DOCKER_API_PORT" \
            --host="unix:///var/run/docker.sock" \
            --tlsverify \
            --tlscacert="$TLS_CERT_DIR/ca.pem" \
            --tlscert="$TLS_CERT_DIR/server-cert.pem" \
            --tlskey="$TLS_CERT_DIR/server-key.pem" \
            > "$DOCKER_DAEMON_LOG_FILE" 2>&1 &
    else
        # Start Docker daemon without TLS (insecure)
        print_warning "Starting Docker daemon WITHOUT TLS on $DOCKER_API_HOST:$DOCKER_API_PORT"
        print_warning "This is INSECURE and should only be used in trusted environments!"
        nohup dockerd \
            --host="tcp://$DOCKER_API_HOST:$DOCKER_API_PORT" \
            --host="unix:///var/run/docker.sock" \
            > "$DOCKER_DAEMON_LOG_FILE" 2>&1 &
    fi
    
    # Save PID
    echo $! > "$DOCKER_DAEMON_PID_FILE"
    
    # Wait a moment and check if it started successfully
    sleep 3
    if is_docker_running; then
        print_status "Docker API daemon started successfully (PID: $(cat $DOCKER_DAEMON_PID_FILE))"
        print_status "Logs: tail -f $DOCKER_DAEMON_LOG_FILE"
        
        if [[ "$DOCKER_API_PORT" == "2376" ]]; then
            print_status "Client certificates available at: $TLS_CERT_DIR"
            print_status "To connect: docker --tlsverify --tlscacert=$TLS_CERT_DIR/ca.pem --tlscert=$TLS_CERT_DIR/client-cert.pem --tlskey=$TLS_CERT_DIR/client-key.pem -H=tcp://$DOCKER_API_HOST:$DOCKER_API_PORT version"
        fi
    else
        print_error "Failed to start Docker daemon. Check logs: $DOCKER_DAEMON_LOG_FILE"
        exit 1
    fi
}

# Function to stop Docker daemon
stop_docker_daemon() {
    if ! is_docker_running; then
        print_warning "Docker API daemon is not running"
        return 0
    fi

    local pid=$(cat "$DOCKER_DAEMON_PID_FILE")
    print_status "Stopping Docker API daemon (PID: $pid)"
    
    # Send SIGTERM first
    kill "$pid" 2>/dev/null || true
    
    # Wait up to 10 seconds for graceful shutdown
    local count=0
    while [[ $count -lt 10 ]] && kill -0 "$pid" 2>/dev/null; do
        sleep 1
        ((count++))
    done
    
    # Force kill if still running
    if kill -0 "$pid" 2>/dev/null; then
        print_warning "Force killing Docker daemon"
        kill -9 "$pid" 2>/dev/null || true
    fi
    
    rm -f "$DOCKER_DAEMON_PID_FILE"
    print_status "Docker API daemon stopped"
}

# Function to show Docker daemon status
docker_status() {
    if is_docker_running; then
        local pid=$(cat "$DOCKER_DAEMON_PID_FILE")
        print_status "Docker API daemon is running (PID: $pid)"
        print_status "Listening on: $DOCKER_API_HOST:$DOCKER_API_PORT"
        print_status "Log file: $DOCKER_DAEMON_LOG_FILE"
        
        if [[ "$DOCKER_API_PORT" == "2376" ]]; then
            print_status "TLS enabled - certificates in: $TLS_CERT_DIR"
        else
            print_warning "TLS disabled - connection is INSECURE"
        fi
    else
        print_warning "Docker API daemon is not running"
    fi
}

# Function to show usage
usage() {
    cat << EOF
Usage: $0 [COMMAND] [OPTIONS]

Commands:
    apply       Apply iptables rules to restrict Docker API access (default)
    show        Show current iptables rules
    cleanup     Remove all Docker API security rules
    test        Test Docker API connectivity
    backup      Backup current iptables rules
    start       Start Docker daemon with API enabled
    stop        Stop Docker daemon
    restart     Restart Docker daemon
    status      Show Docker daemon status
    logs        Show Docker daemon logs
    full-setup  Complete setup (start daemon + apply iptables rules)

Environment Variables:
    DOCKER_API_PORT    Docker API port (default: 2376)
    DOCKER_API_HOST    Docker API bind address (default: 0.0.0.0)
    ALLOWED_IPS        Space-separated list of allowed IPs/CIDRs
                      (default: "127.0.0.1 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16")

Examples:
    # Complete setup (recommended)
    $0 full-setup

    # Start Docker daemon only
    $0 start

    # Apply iptables rules with default settings
    $0 apply

    # Apply rules with custom IPs
    ALLOWED_IPS="192.168.1.100 10.0.0.0/24" $0 apply

    # Start insecure Docker API (no TLS)
    DOCKER_API_PORT=2375 $0 start

    # Show current rules and daemon status
    $0 show
    $0 status

    # Test connectivity
    $0 test

    # View Docker daemon logs
    $0 logs

    # Clean up everything
    $0 cleanup
    $0 stop

EOF
}

# Main execution
main() {
    local command=${1:-"apply"}
    
    case "$command" in
        "apply")
            check_root
            print_status "Starting Docker API security configuration"
            print_status "Docker API Port: $DOCKER_API_PORT"
            print_status "Allowed IPs: $ALLOWED_IPS"
            
            # Backup current rules
            backup_file=$(backup_iptables)
            print_status "Backup saved: $backup_file"
            
            # Apply security rules
            create_chain
            add_allowed_ips
            add_drop_rule
            apply_chain
            
            print_status "Docker API security rules applied successfully!"
            print_warning "Make sure to test connectivity before disconnecting"
            show_rules
            ;;
        "show")
            show_rules
            ;;
        "cleanup")
            check_root
            cleanup
            print_status "Docker API security rules removed"
            ;;
        "test")
            test_connection
            ;;
        "backup")
            check_root
            backup_file=$(backup_iptables)
            print_status "Backup completed: $backup_file"
            ;;
        "start")
            check_root
            start_docker_daemon
            ;;
        "stop")
            check_root
            stop_docker_daemon
            ;;
        "restart")
            check_root
            stop_docker_daemon
            sleep 2
            start_docker_daemon
            ;;
        "status")
            docker_status
            ;;
        "logs")
            if [[ -f "$DOCKER_DAEMON_LOG_FILE" ]]; then
                tail -f "$DOCKER_DAEMON_LOG_FILE"
            else
                print_error "Docker daemon log file not found: $DOCKER_DAEMON_LOG_FILE"
                print_error "Start the Docker daemon first with: $0 start"
            fi
            ;;
        "full-setup")
            check_root
            print_status "Starting complete Docker API setup"
            print_status "Docker API Port: $DOCKER_API_PORT"
            print_status "Docker API Host: $DOCKER_API_HOST"
            print_status "Allowed IPs: $ALLOWED_IPS"
            
            # Start Docker daemon
            start_docker_daemon
            
            # Apply iptables rules
            backup_file=$(backup_iptables)
            print_status "Backup saved: $backup_file"
            
            create_chain
            add_allowed_ips
            add_drop_rule
            apply_chain
            
            print_status "Complete Docker API setup finished!"
            print_status "Docker daemon is running with API security enabled"
            show_rules
            docker_status
            ;;
        "help"|"-h"|"--help")
            usage
            ;;
        *)
            print_error "Unknown command: $command"
            usage
            exit 1
            ;;
    esac
}

# Handle script interruption
trap 'print_error "Script interrupted. You may need to run cleanup if rules were partially applied."' INT TERM

# Run main function with all arguments
main "$@"
