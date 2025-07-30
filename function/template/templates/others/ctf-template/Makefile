BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
SUDO ?= 
WATCHER_PID_FILE=.gzctf/.watcher.pid
WATCHER_LOG_FILE=.gzctf/watcher.log

sync:
	${SUDO} ${BIN} gzcli --sync

sync-and-update-game:
	${SUDO} ${BIN} gzcli --sync --update-game

setup: setup-traefik-network
	@echo "Setting up CTF environment with Traefik..."
	@echo "External network 'traefik' created"
	@echo "You can now run 'make platform-up' to start the platform"

platform-up: setup-traefik-network
	@echo "Starting CTF platform with docker compose..."
	(cd .gzctf && ${SUDO} docker compose up -d)

platform-down:
	@echo "Stopping CTF platform with docker compose..."
	(cd .gzctf && ${SUDO} docker compose down)

gzcli-start: setup-traefik-network
	${SUDO} ${BIN} gzcli --run-script start

gzcli-stop:
	${SUDO} ${BIN} gzcli --run-script stop

watch:
	@if [ -f ${WATCHER_PID_FILE} ]; then \
		PID=$$(cat ${WATCHER_PID_FILE}); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "Watcher is already running (PID: $$PID)"; \
			echo "Use 'make watch-stop' to stop it first, or 'make watch-logs' to view logs"; \
			exit 1; \
		else \
			echo "Removing stale PID file..."; \
			rm -f ${WATCHER_PID_FILE}; \
		fi \
	fi
	@echo "Starting CTFify watcher with git pull..."
	@nohup ${SUDO} ${BIN} gzcli --watch --watch-git-pull > ${WATCHER_LOG_FILE} 2>&1 & echo $$! > ${WATCHER_PID_FILE}
	@echo "Watcher started in background (PID: $$(cat ${WATCHER_PID_FILE}))"
	@echo "Logs: tail -f ${WATCHER_LOG_FILE}"

watch-stop:
	@if [ -f ${WATCHER_PID_FILE} ]; then \
		PID=$$(cat ${WATCHER_PID_FILE}); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "Stopping watcher (PID: $$PID)..."; \
			kill $$PID; \
			rm -f ${WATCHER_PID_FILE}; \
			echo "Watcher stopped."; \
		else \
			echo "Watcher process not running."; \
			rm -f ${WATCHER_PID_FILE}; \
		fi \
	else \
		echo "No watcher PID file found."; \
	fi

watch-status:
	@if [ -f ${WATCHER_PID_FILE} ]; then \
		PID=$$(cat ${WATCHER_PID_FILE}); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "Watcher is running (PID: $$PID)"; \
			echo "Logs: tail -f ${WATCHER_LOG_FILE}"; \
		else \
			echo "Watcher is not running (stale PID file)"; \
			rm -f ${WATCHER_PID_FILE}; \
		fi \
	else \
		echo "Watcher is not running"; \
	fi

watch-logs:
	@if [ -f ${WATCHER_LOG_FILE} ]; then \
		tail -f ${WATCHER_LOG_FILE}; \
	else \
		echo "No watcher log file found. Start the watcher first with 'make watch'"; \
	fi

register-all-user:
	${SUDO} ${BIN} gzcli --create-teams ${CSV}

send-email:
	${SUDO} ${BIN} gzcli --create-teams-and-send-email ${CSV}

flush-cache:
	(cd .gzctf && ${SUDO} docker compose exec -uroot cache redis-cli FLUSHALL)

setup-traefik-network:
	${SUDO} docker network create traefik || true

traefik-logs:
	(cd .gzctf && ${SUDO} docker compose logs -f traefik)

traefik-dashboard:
	@echo "Traefik dashboard should be available at: http://localhost:8080"
	@echo "Note: Make sure to configure dashboard access in production"

restart-traefik:
	(cd .gzctf && ${SUDO} docker compose restart traefik)

ssl-status:
	@echo "SSL certificates are automatically managed by Traefik"
	@echo "Check Traefik logs for SSL certificate status: make traefik-logs"

platform-logs:
	(cd .gzctf && ${SUDO} docker compose logs -f)

db-logs:
	(cd .gzctf && ${SUDO} docker compose logs -f db)

gzctf-logs:
	(cd .gzctf && ${SUDO} docker compose logs -f gzctf)

platform-clean:
	@echo "Stopping all services and cleaning up..."
	(cd .gzctf && ${SUDO} docker compose down -v) || true
	@echo "Note: External 'traefik' network is preserved for other services"

# Convenient aliases
up: platform-up
down: platform-down
clean: platform-clean

help:
	@echo "CTFify Makefile - Traefik Edition"
	@echo ""
	@echo "Setup & Platform Management:"
	@echo "  setup               - Initial setup (creates traefik network)"
	@echo "  platform-up         - Start platform with docker compose up -d"
	@echo "  platform-down       - Stop platform with docker compose down"
	@echo "  platform-clean      - Stop and remove all containers/volumes"
	@echo ""
	@echo "GZCli Management (alternative):"
	@echo "  gzcli-start         - Start platform using gzcli scripts"
	@echo "  gzcli-stop          - Stop platform using gzcli scripts"
	@echo ""
	@echo "Challenge Sync:"
	@echo "  sync                - Sync challenges"
	@echo "  sync-and-update-game - Sync challenges and update game"
	@echo ""
	@echo "File Watcher:"
	@echo "  watch               - Start file watcher with git pull"
	@echo "  watch-stop          - Stop the watcher"
	@echo "  watch-status        - Check watcher status"
	@echo "  watch-logs          - View watcher logs"
	@echo ""
	@echo "User Management:"
	@echo "  register-all-user   - Register users from CSV"
	@echo "  send-email          - Create teams and send emails from CSV"
	@echo ""
	@echo "Traefik Operations:"
	@echo "  setup-traefik-network - Create external traefik network"
	@echo "  traefik-logs        - View Traefik logs"
	@echo "  traefik-dashboard   - Show Traefik dashboard info"
	@echo "  restart-traefik     - Restart Traefik service"
	@echo "  ssl-status          - Show SSL certificate status"
	@echo ""
	@echo "Logging:"
	@echo "  platform-logs       - View all platform service logs"
	@echo "  db-logs             - View database logs only"
	@echo "  gzctf-logs          - View GZCTF application logs only"
	@echo ""
	@echo "Utilities:"
	@echo "  flush-cache         - Clear Redis cache"
	@echo ""
	@echo "Convenient Aliases:"
	@echo "  up                  - Alias for platform-up"
	@echo "  down                - Alias for platform-down"
	@echo "  clean               - Alias for platform-clean"

