#!/bin/bash

while true; do
	./client
	if [ ! -e .restart ]; then
		break
	fi
done

