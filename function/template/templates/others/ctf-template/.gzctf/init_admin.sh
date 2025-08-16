#!/bin/sh
sudo docker compose exec db psql --user postgres -d gzctf -c "UPDATE \"AspNetUsers\" SET \"Role\"=3 WHERE \"UserName\"='admin';"
