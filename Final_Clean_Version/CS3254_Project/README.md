# CS3254_Project Phase 4
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

## Client Command Example
    locally
    go run frontend.go --listen=8095 --backend=:8090,:8091,:8092,:8093,:8094

    remote
    go run frontend.go --listen=8095 --backend=172.1.12:8090,172.1.13:8091,....

## Server Command Example
    locally
    go run backend.go --listen=8090 --id=:8090 --backend=:8091,:8092,:8093,....

    remote
    go run backend.go --listen=8090 --id=172.1.12:8090 --backend=172.1.13:8091,172.1.14:8092

## Flags
    1. --listen <port> (allowed in both frontend & backend)
    2. --backend ipaddress,ipaddress,... (This is a common separated list, do not put spaces)

        ip address format: <ip>:<port> (remote) // :<port> (local)

        backend should not include the current node's IP.

    3. --id <ip>:<port> (used for backend) This is the IP Address of the current node (the node you are about to run the code on).
        
        Clarification: it's basically the address that other nodes use to connect to you.

## Phase 4 Questions
    Implementation: Raft
    Program Resilience
    1. The program does pass all of the test cases described in the project phase 4 to my knowledge. The application functions normally despite after the leader fails without loss of data. When a quorum of the cluster fails, 
    the client's request simply timeout after 5 seconds (preset timer) and gives up
    2. The program follows the Raft algorithm. When the leader fails, all followers will timeout eventually and attempt to become the new leader. The votes are only granted to those nodes that have the most up to date log, follows two criterias desribed by Raft, which utilizes the last log entry term number and the length of the logs.

        Candidates call RequestVote onto all other replicas to get their votes until they have a majority vote and become leader or when they get an AppendEntries from the elected leader and become a follower

        Things that can go wrong.
        During the sync process, if the leader gets flooded by requests, and dies, the sync process for the replicas might not get the requests that were queued in the leader's log.
    
    Important Design Decisions
    1. I utilized the AppendEntries RPC to replicate the log. The replication strategy follows what is described in the Raft research paper. The leader keeps track of nextIndex for each replica, which is effectively the index of the next entry that the replica needs from the leader. 
    
    Under normal raft, a successful AppendEntries would increment the nextIndex for that replica and we will send the next entry accordingly. Each failed AppendEntries will indicate an inconsistency, so we go as far back in the log as needed to the point where the index were still consistent and then fix the inconsistency accordingly. By sending every needed entry one by one, and incrementing the nextIndex.

    I made the slight modification to send all log entries from nextIndex to the end of the leader's log and directly set the nextIndex to be the last index of the leader's log to reduce the number of messages needed to replicate the logs. This is safe to do once we decrement nextIndex to the point where the logs were last consistent. Every log entry after the nextIndex on the follower's log is considered invalid, thus it is safe to overwrite them with the leader's entries.

    This replication strategy fits very nicely to the Raft algorithm since it still only relies on the AppendEntries RPC, reducing the complexity of the implementation.

    2. Pros:
            It's simple since it still utilizes the same mechancism used for heartbeats and sending new log entries. 

            It also reduced the number of messages it takes to replicate the log from the original Raft algorithm
        Cons:
            The amount of data that I am sending over the network could be fairly large if we are forced to replicate or reproduce the entire log of the leader. (Since our log isn't stored on disk)

            Changes to the AppendEntries RPC could very easily break a lot of different mechancisms that depend on the behavior of the RPC function
    
    3. Each node basically acts like a state machine. It handle requests as soon as it receives them and modifies the state accordingly. The main raft instance runs in a separate thread in comparison to the threads that handle the RPC calls, election process, client request handling, etc. Each event is handled by a thread and they all work with the same instance of raft so other threads would know about any state changes (follower, candidate, leader changes).


## Resources / Tools Used
1. Golang Iris Framework
2. Golang Version 1.13
3. Bootstrap 4 for styling (imported via CDN)
4. net/rpc