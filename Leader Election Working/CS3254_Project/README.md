# CS3254_Project Phase 3
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

## Phase 3 Questions
'''
1. How did you provide safe, concurrent, performant access to your data store? What pros or cons
does your approach have? What other approaches did you consider?
What performance metrics do you consider important and how did you measure them? Include
relevant reports from Vegeta. You may also include reports from other versions of your Phase 3,
demonstrating discarded synchronization strategies.

I locked the whole database with a mutex during read and writes. 
The main pro is that it is simple. The main con is that it impacts performance significantly when the backend server is under load since it limits concurrency.
Another approach I considered to boost performance was to have multiple mutex locks per piece of data in the database (in my case, per product item)
The main pro is that it allows higher levels of concurrency as multiple threads can access the database and work on different parts in the same time, boosting performance. A con of this approach would be having the track and manage many locks as the database grows in size.

I consider the mean response time per request to be an important performance metric since it would correlate with the user experience and how fast a request gets handled. With Vegeta, I was able to measure the response time and it generally got worse linearly as more load was put on the backend.


2. How did you implement the failure detector? What timeouts did you use? What other approaches
did you consider?

Failure detection style: Heartbeats

I had dedicated a port on the backend :8091 to be for heartbeats. Frontend server would periodically (set to 1 minute)
try to connect / ping the backend server on port 8091. If the backend is up, it will simply accept the connection and close it right away.
The main disadvantage of this approach is scale since it produces a lot of network traffic just for heartbeats.
Another approach I considered was simply a ping to the backend whenever the frontend needs to talk to the backend. It would ping first to check if the backend
is alive. Advantage of this approach is much less traffic and pings are only sent when the frontend needs to know if the backend is alive.

'''

## Resources / Tools Used
1. Golang Iris Framework
2. Golang Version 1.13
3. Bootstrap 4 for styling (imported via CDN)
4. net/rpc