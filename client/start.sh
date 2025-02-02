#!/bin/bash

while true; do
	./client
	if [ $? -ne 2 ]; then
		break
	fi
done

