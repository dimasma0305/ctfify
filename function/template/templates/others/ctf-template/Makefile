BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
SUDO ?= 
WATCHER_PID_FILE=.gzctf/.watcher.pid
WATCHER_LOG_FILE=.gzctf/watcher.log

sync:
	${SUDO} ${BIN} gzcli --sync

sync-and-update-game:
	${SUDO} ${BIN} gzcli --sync --update-game

start:
	${SUDO} ${BIN} gzcli --run-script start

stop:
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

install-ssl:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx certbot certonly --webroot -d {{.PublicEntry}} -m dimasmaulana0305@gmail.com --agree-tos --non-interactive --webroot-path /var/www/html/)

link-ssl-config:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx bash -c "rm /etc/nginx/sites-enabled/* && ln -s /etc/nginx/sites-available/{{.PublicEntry}} /etc/nginx/sites-enabled/{{.PublicEntry}}")

reload-nginx:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx nginx -s reload)

local-cert:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx bash -c "mkdir -p /etc/letsencrypt/live/{{.PublicEntry}}/ && openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout /etc/letsencrypt/live/{{.PublicEntry}}/privkey.pem -out /etc/letsencrypt/live/{{.PublicEntry}}/fullchain.pem -subj "/C=US/ST=State/L=City/O=Organization/OU=Unit/CN={{.PublicEntry}}"")

setup-ssl: install-ssl link-ssl-config reload-nginx
setup-local-ssl: local-cert link-ssl-config reload-nginx
reload-ssl-config: link-ssl-config reload-nginx

