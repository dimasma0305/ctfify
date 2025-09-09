BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
SUDO ?= 
WATCHER_PID_FILE=.gzctf/.watcher.pid
GITPULL_PID_FILE=.gzctf/.gitpull.pid
WATCHER_LOG_FILE=.gzctf/watcher.log
PUBLIC_URL="<public-url>"

sync:
	${SUDO} ${BIN} gzcli --sync

sync-and-update-game:
	${SUDO} ${BIN} gzcli --sync --update-game

setup:
	@echo "Setting up CTF environment with Traefik..."
	@echo "Traefik network will be created by docker-compose"
	@echo "You can now run 'make platform-up' to start the platform"

platform-up:
	@echo "Starting CTF platform with docker compose..."
	(cd .gzctf && ${SUDO} docker compose up -d)

platform-down:
	@echo "Stopping CTF platform with docker compose..."
	(cd .gzctf && ${SUDO} docker compose down)

gzcli-start:
	${SUDO} ${BIN} gzcli --run-script start

gzcli-stop:
	${SUDO} ${BIN} gzcli --run-script stop

watch:
	${SUDO} ${BIN} gzcli --watch --watch-git-pull --watch-git-pull-interval 30s --watch-git-repository .

watch-stop:
	${SUDO} ${BIN} gzcli --watch-stop

watch-status:
	${SUDO} ${BIN} gzcli --watch-status

watch-logs:
	${SUDO} ${BIN} gzcli --watch-logs

# Watcher Client Commands (for interacting with running daemon)
watcher-status:
	${SUDO} ${BIN} gzcli --watcher-client status

watcher-list:
	${SUDO} ${BIN} gzcli --watcher-client list

watcher-logs:
	${SUDO} ${BIN} gzcli --watcher-client logs --watcher-limit 50

watcher-logs-all:
	${SUDO} ${BIN} gzcli --watcher-client logs --watcher-limit 200

watcher-live-logs:
	${SUDO} ${BIN} gzcli --watcher-client live-logs --watcher-limit 50 --watcher-interval 2s

watcher-live-logs-fast:
	${SUDO} ${BIN} gzcli --watcher-client live-logs --watcher-limit 50 --watcher-interval 1s

watcher-metrics:
	${SUDO} ${BIN} gzcli --watcher-client metrics

watcher-executions:
	${SUDO} ${BIN} gzcli --watcher-client executions --watcher-limit 100

# Challenge-specific watcher operations (requires CHALLENGE= parameter)
watcher-restart-challenge:
ifndef CHALLENGE
	$(error CHALLENGE is not set. Use: make watcher-restart-challenge CHALLENGE=challenge-name)
endif
	${SUDO} ${BIN} gzcli --watcher-client restart --watcher-challenge $(CHALLENGE)

watcher-stop-script:
ifndef CHALLENGE
	$(error CHALLENGE is not set. Use: make watcher-stop-script CHALLENGE=challenge-name SCRIPT=script-name)
endif
ifndef SCRIPT
	$(error SCRIPT is not set. Use: make watcher-stop-script CHALLENGE=challenge-name SCRIPT=script-name)  
endif
	${SUDO} ${BIN} gzcli --watcher-client stop-script --watcher-challenge $(CHALLENGE) --watcher-script $(SCRIPT)

watcher-challenge-executions:
ifndef CHALLENGE
	$(error CHALLENGE is not set. Use: make watcher-challenge-executions CHALLENGE=challenge-name)
endif
	${SUDO} ${BIN} gzcli --watcher-client executions --watcher-challenge $(CHALLENGE) --watcher-limit 50

register-all-user:
	${SUDO} URL=${PUBLIC_URL} ${BIN} gzcli --create-teams ${CSV}

register-all-user-and-send-email:
	${SUDO} URL=${PUBLIC_URL} ${BIN} gzcli --create-teams-and-send-email ${CSV}

flush-cache:
	(cd .gzctf && ${SUDO} docker compose exec -uroot cache redis-cli FLUSHALL)

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
	@echo "Note: Traefik network will be recreated automatically if needed"

# Convenient aliases
up: platform-up
down: platform-down
clean: platform-clean

help:
	@echo "CTFify Makefile - Traefik Edition"
	@echo ""
	@echo "Setup & Platform Management:"
	@echo "  setup               - Initial setup (no manual network creation)"
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
	@echo "Watcher Client (interact with running daemon):"
	@echo "  watcher-status      - Get watcher daemon status"
	@echo "  watcher-list        - List watched challenges"
	@echo "  watcher-logs        - View database logs (last 50)"
	@echo "  watcher-logs-all    - View database logs (last 200)"
	@echo "  watcher-live-logs   - Stream live database logs (2s refresh)"
	@echo "  watcher-live-logs-fast - Stream live database logs (1s refresh)"
	@echo "  watcher-metrics     - View script execution metrics"
	@echo "  watcher-executions  - View script execution history"
	@echo ""
	@echo "Watcher Challenge Operations:"
	@echo "  watcher-restart-challenge CHALLENGE=name - Restart challenge"
	@echo "  watcher-stop-script CHALLENGE=name SCRIPT=name - Stop interval script"
	@echo "  watcher-challenge-executions CHALLENGE=name - View challenge executions"
	@echo ""
	@echo "User Management:"
	@echo "  register-all-user   - Register users from CSV"
	@echo "  send-email          - Create teams and send emails from CSV"
	@echo ""
	@echo "Traefik Operations:"
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
