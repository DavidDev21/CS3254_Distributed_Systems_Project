# CS3254_Project
Project for Parallel and Distributed Systems Course

### Author: David Zheng (dz1063)

### Status: Working

## Setup and Run
1. Put this folder into $GOPATH/src
2. cd into $GOPATH/src
3. run "go get CS3254_Project/client" and "go get CS3254_Project/server"
4. cd into the CS3254_Project directory
5. cd into "server" and run "go run backend.go" to start the backend server
6. cd into "client" and run "go run frontend.go" to start the frontend server

The order for step 5 and 6 doesn't matter, but both should be running prior to using the web app.

## Flags
1. --listen <port> (allowed in both frontend & backend)
2. --backend <ip>:<port> or --backend :<port> (allowed only in frontend)

## Design
'''
I utilized net/rpc package to allow the frontend to do a remote procedure calls to the backend
server in order to query the database.

I used a tcp connection between the frontend server and backend server. 
The main advantage of using a tcp connection between the two servers is guaranteed delivery.
The frontend would not be able to serve the data properly if packets of data are loss.
The main disadvantage of the current design would be concurrency issues if there are multiple frontend servers, working with one backend server.
'''

## Resources / Tools Used
1. Golang Iris Framework
2. Golang Version 1.13
3. Bootstrap 4 for styling (imported via CDN)
4. net/rpc