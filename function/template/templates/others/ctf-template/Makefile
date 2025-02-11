BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
sync:
	sudo ${BIN} gzcli --sync
sync-and-updage-game:
	sudo ${BIN} gzcli --sync --update-game
start:
	sudo ${BIN} gzcli --run-script start
stop:
	sudo ${BIN} gzcli --run-script stop
register-all-user:
	sudo ${BIN} gzcli --create-teams ${CSV}
send-email:
	sudo ${BIN} gzcli --create-teams-and-send-email ${CSV}
install-ssl:
	(cd .gzcf && sudo docker compose exec nginx certbot --nginx -d {{.PublicEntry}})
