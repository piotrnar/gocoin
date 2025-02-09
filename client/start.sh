#!/bin/bash

while true; do
	./client
	if [ $? -ne 66 ]; then
		break
	fi
done

