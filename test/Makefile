BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
SUDO ?= 

sync:
	${SUDO} ${BIN} gzcli --sync

sync-and-update-game:
	${SUDO} ${BIN} gzcli --sync --update-game

start:
	${SUDO} ${BIN} gzcli --run-script start

stop:
	${SUDO} ${BIN} gzcli --run-script stop

register-all-user:
	${SUDO} ${BIN} gzcli --create-teams ${CSV}

send-email:
	${SUDO} ${BIN} gzcli --create-teams-and-send-email ${CSV}

install-ssl:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx certbot --nginx -d playground.tcp1p.team)

link-ssl-config:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx bash -c "rm /etc/nginx/sites-enabled/* && ln -s /etc/nginx/sites-available/playground.tpcp1.team /etc/nginx/sites-enabled/playground.tpcp1.team")

reload-nginx:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx nginx -s reload)

local-cert:
	(cd .gzctf && ${SUDO} docker compose exec -uroot nginx bash -c "mkdir -p /etc/letsencrypt/live/playground.tcp1p.team/ && openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout /etc/letsencrypt/live/playground.tcp1p.team/privkey.pem -out /etc/letsencrypt/live/playground.tcp1p.team/fullchain.pem -subj "/C=US/ST=State/L=City/O=Organization/OU=Unit/CN=playground.tcp1p.team"")

setup-ssl: install-ssl link-ssl-config reload-nginx
setup-local-ssl: local-cert link-ssl-config reload-nginx
reload-ssl-config: link-ssl-config reload-nginx

