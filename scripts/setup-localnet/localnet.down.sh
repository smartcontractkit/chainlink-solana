#!/usr/bin/env bash

echo "Cleaning up test validator container.."

echo "Checking for existing 'chainlink-solana.test-validator' docker container..."
dpid=`docker ps -a | grep chainlink-solana.test-validator | awk '{print $1}'`;
if [ -z "$dpid" ]
then
    echo "No docker test validator container running.";
else
    docker kill $dpid;
    docker rm $dpid;
fi

echo "Cleanup finished."