BIN=`which ctfify`
CSV="https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv"
sync:
	sudo ${BIN} gzcli --sync
start:
	sudo ${BIN} gzcli --run-script start
stop:
	sudo ${BIN} gzcli --run-script stop
register-all-user:
	sudo ${BIN} gzcli --create-teams ${CSV}
send-email:
	sudo ${BIN} gzcli --create-teams-and-send-email ${CSV}
