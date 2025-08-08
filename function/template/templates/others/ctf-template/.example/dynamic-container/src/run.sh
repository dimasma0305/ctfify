#!/bin/sh
socat tcp-l:8011,reuseaddr,fork exec:"python3 chall.py"
