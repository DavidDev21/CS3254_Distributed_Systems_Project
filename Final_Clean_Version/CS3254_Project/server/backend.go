/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 4 (Backend)
	backend.go will serve as the starting point for the backend server

*/
package main

import ("fmt"
		"flag"
		"net"
		"net/rpc"
		"os"
		"errors"
		"sync"
		"strings"
		"time"
		"math/rand"
		CO "CS3254_Project/custom_objects")

// Note: unexported methods aren't registered for RPC, (lowercase methods)

// Performs RPC Dial to address under the indicated network protocol
// Exits the program if there was an error
func DialWithCheck(protcol, address string) (*rpc.Client, error) {
	// fmt.Println("address: ", address)
	conn, err := rpc.Dial(protcol, address)

	if (err != nil) {
		//fmt.Println(err.Error())
		return conn, errors.New("Connection To Replica Failed")
	}

	return conn, nil
}

func minInt(x, y int) int {
	if (x > y) {
		return y
	}
	return y
}

// Raft Stuff
const FOLLOWER_STATE = "follower"
const LEADER_STATE = "leader"
const CANDIDATE_STATE = "candidate"
const HEARTBEAT_PERIOD = time.Duration(100) * time.Millisecond // Should be less than the min timeout time
// Time out range is based off the suggested values in the raft paper
const TIMEOUT_RANGE_START = 150
const TIMEOUT_RANGE_END = 300

// Raft Channels
var startElection chan bool // start getting votes
var convertFollower chan bool // convert to follower
var heartbeatSignal chan bool // reset timeout
var killHeart chan bool // signals to the heartbeat thread on the leader to stop
var startSignal chan bool // start a new state

// Raft Mutex
var RaftMutex = &sync.Mutex{}
var voteLock = &sync.Mutex{}

// Timer
var timeout int
var timer <-chan time.Time

// Sets the timer for random time between the start and end range
func SetTimer(start, end int) {
	timeout = rand.Intn(end) + start
	timer = time.After(time.Duration(timeout) * time.Millisecond)
}

// We need the arguments that come with the commands
// In order to replicate the state of the database
type CommandArgs struct {
	ProductID int
	ProductInfo CO.Product
}

type CommandResponse struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

type SendCommandArgs struct {
	Command string // what command to execute
	Args CommandArgs // the arguments for the command
}

type SendCommandResponse struct {
	LeaderID string // to tell the client who the leader is
	Success bool // whether the command was successful
	Response CommandResponse // the result of the command being executed
}

type LogEntry struct {
	TermNumber int
	Command string
	Args CommandArgs
}

type Raft struct {
	ID string // to make it easier to ID ourselves
	currentTerm int
	votedFor string // Who did we vote for during election
	committedIndex int // the highest Log Entry that was known to be committed, i.e, to have been executed on this machine
	log []LogEntry // Holds the termNumber, Command, and Command Args from the client

	nextIndex []int // track of what the next log entry should be sent to i'th follower, should be 1 to 1 with replicas[]
	state string // Current State of the Raft Instance

	quorumSize int // the minimum quorum we need given our initial configuration
	leaderID string // The leader's ID (their IP address)
	replicas []string // The address of all the backend replcias
}

// Raft Infrastructure Code
// Setsup initial state of raft node
func (self *Raft) raftSetup(nodeID string, replicas []string) {
	self.state = FOLLOWER_STATE
	self.ID = nodeID
	self.leaderID = "" // We don't know who the leader is initially
	self.currentTerm = 1
	self.votedFor = ""
	self.committedIndex = 0
	self.replicas = replicas
	self.quorumSize = (len(replicas)/2) + 1

	self.log = []LogEntry{LogEntry{TermNumber: 0, Command: "",}}

	self.nextIndex = make([]int, len(self.replicas))

	for i := 0; i < len(self.nextIndex) ; i++ {
		self.nextIndex[i] = len(self.log)
	}
}

// Simply empties out anything in the channel
func cleanChannel(c chan bool) {
	lenChan := len(c)
	for i := 0; i < lenChan; i++ {
		<-c
	}
}

func (self *Raft) printState() {
	RaftMutex.Lock()
	fmt.Println("ID: ", self.ID)
	fmt.Println("State: ", self.state)
	fmt.Println("LeaderID: ", self.leaderID)
	fmt.Println("currentTerm: ", self.currentTerm)
	fmt.Println("votedFor: ", self.votedFor)
	fmt.Println("committedIndex: ", self.committedIndex)
	fmt.Println()
	fmt.Println("Log: ", self.log)
	fmt.Println("NextIndex: ", self.nextIndex)
	fmt.Println()
	RaftMutex.Unlock()
}

// Used to check if a quorum of the nodes have their logs at least up to logEntryIndex 
// pass in self.commitedIndex to see if all nodes have commited up to a given point
func (self *Raft) checkQuorum (logEntryIndex int) bool {
	numConsistent := 1

	// don't need mutex since replicas and quorumSize never change
	for _, replicaNextEntry := range self.nextIndex {
		if (replicaNextEntry - 1 >= logEntryIndex) {
			numConsistent += 1
		}
	}

	if (numConsistent >= self.quorumSize) {
		return true
	}

	return false
}

// This lets the raft instance to apply the given log entry and execute the commands
// to the database (our global variable data store)
// These commands are straight from project phase 2
// With light modification to take in a LogEntry type
func (self *Raft) applyLogEntry(entry LogEntry, res *CommandResponse) {
	switch entry.Command {
	case "EditProduct":
		fmt.Println("ApplyLogEntry: EditProduct")
		EditProduct(entry.Args, res)
	case "AddProduct":
		fmt.Println("ApplyLogEntry: AddProduct")
		AddProduct(entry.Args, res)
	case "DeleteProduct":
		fmt.Println("ApplyLogEntry: DeleteProduct")
		DeleteProduct(entry.Args, res)
	case "GetProductList":
		fmt.Println("ApplyLogEntry: GetProductList")
		GetProductList(entry.Args, res)
	case "GetProduct":
		fmt.Println("ApplyLogEntry: GetProduct")
		GetProduct(entry.Args, res)
	}
}


// Client Interaction RPCs
// Client's will send all their requests via SendCommand RPC
// This gives the opportunity for the nodes in the Raft cluster to handle a client request
// A follower or candidate will simply tell the client who it thinks the leader is
// The leader will handle the requests by appending the request to its log
// and then committed and return the result of the request once there is a quorum that has the log entry as well

// Note: if the leader does not have a quorum, the leader will simply hang and wait
// This should be fine since we shouldn't be processing more requests if a quorum of the cluster is down.
func (self *Raft) SendCommand(req SendCommandArgs, res *SendCommandResponse) error {
	RaftMutex.Lock()
	// If we are not the leader, inform the client
	if (self.state != LEADER_STATE) {
		res.LeaderID = self.leaderID
		res.Success = false
		RaftMutex.Unlock()
		return nil
	}
	currentCommittedIndex := self.committedIndex

	// Check if you have a quorum of activeFollowers who are up to date with you
	// (This could not be true even if you have connections to a quorum fo activeFollowers since a follower could be syncing up)
	// if not, reject the request
	if (self.checkQuorum(currentCommittedIndex) == false) {
		res.LeaderID = self.leaderID
		res.Success = false
		RaftMutex.Unlock()
		return nil
	}

	// Append entry to our log
	self.log = append(self.log, LogEntry{TermNumber: self.currentTerm, Command: req.Command, Args: req.Args})
	logEntryIndex := len(self.log)-1 // Used to track when our request is completed

	RaftMutex.Unlock()
	// Wait for all the followers to have the new log entry
	for self.checkQuorum(logEntryIndex) == false && self.committedIndex + 1 < logEntryIndex{

	}

	RaftMutex.Lock()
	// At this point, it should be safe to execute and commit this log entry
	// since all the logs have the entry and we have committed all other entries before us
	// (committedIndex + 1) == logEntryIndex means we have reached this log entry to commit
	// now we can safety execute
	self.applyLogEntry(self.log[logEntryIndex], &res.Response)
	res.LeaderID = self.leaderID
	res.Success = true
	self.committedIndex = self.committedIndex + 1

	RaftMutex.Unlock()
	return nil
}

type AppendEntriesArgs struct {
	Term int
	LeaderID string
	PrevLogIndex int
	PrevLogTerm int
	Entries []LogEntry
	LeaderCommit int
}

type AppendEntriesResponse struct {
	Term int
	Success bool
}

// This is effectively the follower's main method, it just respond to RPC calls from leader
// We get an AppendEntries call from the leader
// We will transition to new state (handled by the individual state functions, runLeader, etc)
func (self *Raft) AppendEntries(req AppendEntriesArgs, res *AppendEntriesResponse) error {

	// Safer to have mutex on the state to prevent issues with other threads accessing / modifying the state, (RequestVote)
	RaftMutex.Lock()
	defer RaftMutex.Unlock()

	// If we get a leader whose term is outdated or behind, then we informed that leader of the new term
	// and force them to become a follower (assuming we are the follower at the moment)
	if (req.Term < self.currentTerm) {
		res.Term = self.currentTerm
		res.Success = false
		return nil
	}

	// Reset Timer now that we got an msg from leader
	heartbeatSignal <- true // only to be picked up by the follower
	// Once we get an AppendEntries from the new leader, forget who we votedFor during the election
	self.votedFor = ""

	fmt.Println("Raft.AppendEntries: Received from ", req.LeaderID)

	firstCondition := self.state == CANDIDATE_STATE
	secondCondition := (self.state == LEADER_STATE && self.currentTerm < req.Term)

	// We are no longer the leader if we were the leader, and also can't be a candidate if a leader was elected
	if (firstCondition || secondCondition) {
		self.state = FOLLOWER_STATE
		
		cleanChannel(convertFollower)
		// This stops a candidate from election (main thread)
		// OR stops leader from being leader (main thread)
		// We can only be in one of three states
		if (len(convertFollower) == 0) {
			convertFollower <- true
		}

		fmt.Println("AppendEntries: Lost Leadership, or Candidate Eligibility")
	}

	self.currentTerm = req.Term // This would cover the case if we are an old leader and need to update
	self.leaderID = req.LeaderID
	fmt.Println("Leader is: ", self.leaderID)

	// Beginning of log replication process

	// If the Leader's log is longer than ours or if the termNumber of the prevlogindex dont match
	// We have an inconsistency, we reject the current set of entries sent by leader
	if (req.PrevLogIndex >= len(self.log) || self.log[req.PrevLogIndex].TermNumber != req.PrevLogTerm) {
		res.Term = self.currentTerm
		res.Success = false
		return nil
	}

	// Otherwise, we can accept the leader's entries
	// Wipe everything after PrevLogIndex, since we established that we matched entries on PrevLogIndex
	// Which by the Log Matching Property of Raft, all entries preceding PrevLogIndex must also match with the leader
	// and also committed safely
	self.log = self.log[:req.PrevLogIndex + 1]

	// Now we append the entries
	self.log = append(self.log, req.Entries...)

	// Process the entries that we just got from leader up to the point the leader committed
	// basically syncing up to the leader
	// This has to be a < because the follower will always be one behind
	// It will only commit the entry if the leader commits (which leaderCommit will > than followerCommit)
	// Since leader increments committedIndex once it gets a quorum of followers that has the entry
	// then the follower gets known of this in the next AppendEntries
	for i := self.committedIndex; i < req.LeaderCommit; i++ {
		heartbeatSignal <- true
		self.applyLogEntry(self.log[i], new(CommandResponse))
		heartbeatSignal <- true
	}


	// We want the minimum between the leader's commit and our last entry
	// because the leader's commit could easily be behind our log
	// or our log is behind the leader's commit
	if (req.LeaderCommit > self.committedIndex) {
		self.committedIndex = minInt(req.LeaderCommit, len(self.log)-1)
	}

	res.Term = self.currentTerm
	res.Success = true

	return nil
}

// Leader Election Methods
type RequestVoteArgs struct {
	Term int
	CandidateID string
	LastLogIndex int
	LastLogTerm int
}

type RequestVoteResponse struct {
	Term int
	VoteGranted bool
}

// Candidate method calls VIA RPC
// Get Vote from follower
/*
	Input Params:
		term = the candidate's current term
		candidateID = the ID of whoever is the candidate requesting ( might be ID or the candidate IP addr, idk)
		lastLogIndex = the index of the last log entry on the candidate's log
		lastLogTerm = the term of the last log entry on the candidate's log
	Response:
		followerTerm (int) = the follower's current term
		voteGranted (bool) = whether the follower granted the vote to the requesting candidate
*/

// self: here should be the receiver end meaning a follower
// Args: term int, candidateID string, lastLogIndex int, lastLogTerm int
func (self *Raft) RequestVote(req RequestVoteArgs, res *RequestVoteResponse) error {
	// Note: the only time we would ever vote for someone is if our term is less than candidate term
	// If candidate claims the same term as us, then we can believe that we already voted for someone in the current term
	// (we would only be the same if we had voted, since we are forced to update our current term to matched the candidate that we are voting for)

	RaftMutex.Lock()
	defer RaftMutex.Unlock()

	// Case: we already voted for someone else in the current term / the candidate's term is behind (covers 2 cases)
	if (self.votedFor != req.CandidateID && req.Term <= self.currentTerm) {
		res.Term = self.currentTerm
		res.VoteGranted = false
		return nil
	}

	// Case: we already voted for the candidate that is requesting for the current term(in the event of network failure)
	if (self.votedFor == req.CandidateID && self.currentTerm == req.Term) {
		res.Term = self.currentTerm
		res.VoteGranted = true
		return nil
	}

	// We grant the vote if we haven't voted for anyone, or if we already voted for candidateID in current term
	// The candidate's log should be at least up to date in respects to our log
	/*
			Snippet from the RAFT paper

			If the logs have last entries with different terms, then
			the log with the later term is more up-to-date. If the logs
			end with the same term, then whichever log is longer is
			more up-to-date.
	*/
	//		- lastLogTerm >= self.lastLogTerm (if our last log entry term is higher, that means the candidate's log doesn't match with our expectations)
	//				Higher term indicates a more true or updated log since high terms require a quorum of followers to elect a leader
	//				Lower terms means the candidate potentially lost connection to the majority of the cluster and was never informed of the new term
	// 		- lastLogIndex >= self.LogIndex (candidate's log should at least be in the same length as ours)

	currentLastIndex := len(self.log)-1

	firstCondition := (req.LastLogTerm > self.log[currentLastIndex].TermNumber)
	secondCondition := (req.LastLogTerm == self.log[currentLastIndex].TermNumber && req.LastLogIndex >= currentLastIndex)

	if (firstCondition || secondCondition) {
		self.votedFor = req.CandidateID
		self.currentTerm = req.Term
		res.Term = self.currentTerm
		res.VoteGranted = true
		return nil
	}

	res.Term = self.currentTerm
	res.VoteGranted = false

	return nil

}

// Follower State

// The follower would simply listen for two types of RPC calls
// AppendEntries and RequestVote and will proceess them accordingly
// The raft instance goes into runFollower() to handle when the timer runs out
// and we need to become the candidate
func (self *Raft) runFollower() {
	SetTimer(TIMEOUT_RANGE_START, TIMEOUT_RANGE_END)
	for self.state == FOLLOWER_STATE {
		select {
		case <- heartbeatSignal:
			SetTimer(TIMEOUT_RANGE_START, TIMEOUT_RANGE_END)
		case <- timer:
			fmt.Println("Follower State: Timed out")
			RaftMutex.Lock()
			self.state = CANDIDATE_STATE
			RaftMutex.Unlock()
			startSignal <- true
			return
		}
	}
}

// Leader State

// With each heartbeat, its an opportunity to sync up the logs between leader and follower
func (self *Raft) leaderHeartbeat(req *AppendEntriesArgs, replicaAddr string, nextIndexPos int) {

	RaftMutex.Lock()
	defer RaftMutex.Unlock()

	// Connect to replica
	conn, err := DialWithCheck("tcp", replicaAddr)

	// If we failed, note it so we can try again later
	if (err != nil) {
		fmt.Println("Raft.AppendEntries (Leader Heartbeat): Failed to establish connection to ", replicaAddr)
		return // move onto the next address to try
	} else {
		res := new(AppendEntriesResponse)
		
		fmt.Println("Calling AppendEntries on ", replicaAddr)
		// RPC Call
		err := conn.Call("Raft.AppendEntries", req, res)
		conn.Close()
	
		if (err != nil) {
			fmt.Println("Raft.AppendEntries (Leader Heartbeat): Error from RPC Call to ", replicaAddr)
			fmt.Println(err)
			return
		}

		// Inconsistency in the follower's log, decrement nextIndex and try again
		// Eventually we will reach a point where nextIndex - 1 is at an entry that matches with the leader's log
		// All Log entries prior to the entry at nextIndex - 1 is guaranteed to be consistent by the Log Matching Property from Raft
		if (res.Success == false && self.nextIndex[nextIndexPos] > 0) {
			self.nextIndex[nextIndexPos] = self.nextIndex[nextIndexPos] - 1
		}

		// Success means we have successfully replicated our log with follower's log
		// move nextIndex for the follower to the end of the log
		if (res.Success == true) {
			self.nextIndex[nextIndexPos] = len(self.log)
		}
	}
	return
}

// This is a separate thread for the leader to have heartbeats in regular intervals
// Also allows us to control the frequency of these messages
func (self *Raft) heartbeatController() {

	for {
		select {
		case <- killHeart:
			// Leader told us to kill the heart
			return
		default:
			fmt.Println("Printing Leader State")
			self.printState()
			// Otherwise we keep beating
			for i, replicaAddr := range self.replicas {
				RaftMutex.Lock()

				req := new(AppendEntriesArgs)
				// This basically setups what the specific replica might need
				// Allows us to send missing entries to the replica
				// and also allows us to dethrone an old leader by sharing our term number
				req.Term = self.currentTerm
				req.LeaderID = self.ID
				req.PrevLogIndex = self.nextIndex[i] - 1
				req.PrevLogTerm = self.log[req.PrevLogIndex].TermNumber
				req.LeaderCommit = self.committedIndex
				req.Entries = self.log[self.nextIndex[i]:] // send everything after nextIndex, basically everything the followr needs

				RaftMutex.Unlock()

				// req.Entries would be empty if nextIndex is the length of the log
				go self.leaderHeartbeat(req, replicaAddr, i)
			}
			// Make the heartbeats more periodical
			time.Sleep(HEARTBEAT_PERIOD)
		}
	}
}

// The Leader methods should be separate into two components
// AppendEntries() which is the made heartbeat mechancism and also sync mechancism (handled by heartbeatController)
// It allows follower to update their logs 
// runLeader simply allows us to jumpstart the AppendEntries heartbeats and keep our timer refreshed for the duration of our leadership
func (self *Raft) runLeader() {
	fmt.Println("Entering Leader")
	// Starts the heartbeat of the leader
	go self.heartbeatController()

	// While we are still the leader
	for self.state == LEADER_STATE {
		// Reset our own timer
		SetTimer(TIMEOUT_RANGE_START, TIMEOUT_RANGE_END)
		select {
		case <-convertFollower:
			fmt.Println("RunLeader: Lost leadership, converting to follower")
			startSignal <- true
			killHeart <- true
			return
		}
	}

	fmt.Println("exiting leadership Heartbeat")
}

// Candidate State

// Simply tries to get a vote from replicaAddr
// If it fails, then it just fails
// We can simply just try to get their vote again next term or in the currenct election
func (self *Raft) getVote(req *RequestVoteArgs, replicaAddr string, numVotes *int, connectionToMake map[string]bool) {
	// Connect to replica
	conn, err := DialWithCheck("tcp", replicaAddr)
	// If we failed, note it so we can try again later
	if (err != nil) {
		fmt.Println("Raft.GetVotes: Failed to establish connection to ", replicaAddr)
		return
	} else {
		fmt.Println("Raft.GetVotes: Established connection to ", replicaAddr)
	
		res := new(RequestVoteResponse)
		// RPC Call
		err := conn.Call("Raft.RequestVote", req, res)

		conn.Close()

		if (err != nil) {
			fmt.Println("Raft.GetVotes: Error from RPC Call to ", replicaAddr)
			fmt.Println(err)
			return
		}

		// We find out our term is outdated, we lose eligibility to be a candidate
		if (res.Term > req.Term) {
			RaftMutex.Lock()
			fmt.Println("Raft.GetVote: outdated term, converting to follower")
			self.currentTerm = res.Term // update our term
			self.state = FOLLOWER_STATE // become a follower when candidate times out
			RaftMutex.Unlock()

			// If no other thread has told candidate convert. Tell them
			if (len(convertFollower) == 0) {
				convertFollower <- true
			}

			return // Exit
		}

		// If we got the vote, great!
		if (res.VoteGranted == true) {
			fmt.Println("Raft.GetVote: We got a vote from ", replicaAddr)
			voteLock.Lock()
			*numVotes = *numVotes + 1
			voteLock.Unlock()
			// indicate we no longer need to call this replica
			// Thread safe since getVote() threads are touching different replicas
			connectionToMake[replicaAddr] = false
			return
		}
	}

	return
}

// Self: here should be the candidate in this case, ourselves (same instance)
// The Candidate will keep trying to get votes from other nodes
// If the election timeout occurs, then we restart the election by re-entering the candidate state
// Or we transition to follower once we hear an AppendEntries from the elected leader
// (this will be signaled by the "convertFollower" channel)
// During the election process, if we failed to get a vote from a node, we will try to contact the node for the duration of the election
func (self *Raft) runCandidate() {
	RaftMutex.Lock()
	// vote for self
	self.votedFor = self.ID
	numVotes := 1
	self.currentTerm = self.currentTerm + 1

	// Setup the arguments we would send to ALL replicas, should be same
	req := new(RequestVoteArgs)

	lastLogIndex := len(self.log)-1
	req.Term = self.currentTerm
	req.CandidateID = self.ID
	req.LastLogIndex = lastLogIndex
	req.LastLogTerm = self.log[lastLogIndex].TermNumber

	RaftMutex.Unlock()

	// We want to clean the channel in case of residual data from previous getVote threads or previous runs
	cleanChannel(convertFollower)
	cleanChannel(startElection)

	fmt.Println("Entering candidate process")

	var connectionToMake = make(map[string]bool)

	for _, replicaAddr := range self.replicas {
		connectionToMake[replicaAddr] = true
	}
	SetTimer(TIMEOUT_RANGE_START, TIMEOUT_RANGE_END)
	
	// If we are not able to contact a quorum initially, then we need to try to get the votes from previously failed attempts
	// And keep trying until someone either informs us of a new leadership, or we timeout and try again
	// While we still think we are the candidate, keep trying to get the votes that you need
	for self.state == CANDIDATE_STATE {
		RaftMutex.Lock()

		// We win if we got enough votes on the first try (assume we were able to contact a quorum)
		// And we didn't get an signal to convert back to follower (if we got an appendentries while in candidate)
		if (numVotes >= self.quorumSize && len(convertFollower) == 0) {
			self.state = LEADER_STATE
			self.leaderID = self.ID // note that we are the leader
			startSignal <- true // Signal the main thread to go to a different state
			RaftMutex.Unlock()
			return // Exit
		}

		// If we still need to retry a connection, then signal to do so
		for _, needToConnect := range connectionToMake {
			if (needToConnect == true) {
				startElection <- true
				break
			}
		}

		RaftMutex.Unlock()
		select {
		case <- startElection:
			// Contact those nodes that we need votes from
			for replicaAddr, needToConnect := range connectionToMake {
				if (needToConnect == true) {
					go self.getVote(req, replicaAddr, &numVotes, connectionToMake)
				}	
			}
			// Add some delay between vote requests, Doesn't make sense to spam messages to a node every iteration
			time.Sleep(HEARTBEAT_PERIOD)
		case <- convertFollower:
			fmt.Println("RunCandidate: Got signal to convert to follower")
			self.state = FOLLOWER_STATE
			// Goes back to runRaft to enter Follower state
			startSignal <- true
			return
		case <-timer:
			fmt.Println("RunCandidate: We timed out during election, restarting")
			// Goes back to runRaft to re-enter Candidate state
			startSignal <- true
			return
		}
	}

	fmt.Println("Exiting Candidate")
	startSignal<-true
	return
}

// Self: is the current Raft instance on this machine
// This helps us transition between different Raft states
// startSignal is used by all states to trigger a new state
func (self *Raft) runRaft(nodeID string, replicas []string) {

	self.raftSetup(nodeID, replicas)
	SetTimer(TIMEOUT_RANGE_START, TIMEOUT_RANGE_END)

	for {
		select {
		// A new state is only triggered or run by startSignal
		// In theory, only one state thread is running at any given moment
		case <-startSignal:
			switch self.state {
			case FOLLOWER_STATE :
				// Most likely will never actually be in here since Follower only listens for Raft RPC calls
				fmt.Println("STATE: I am follower")
				self.runFollower()
			case LEADER_STATE :
				fmt.Println("STATE: I am leader")
				self.runLeader()
			case CANDIDATE_STATE :
				fmt.Println("STATE: I am candidate")
				self.runCandidate()
			}
		}
	}
}

// End of Raft

// Constants
const NUM_THREADS = 5
const DEFAULT_BACKEND_ADDR = "localhost:8090"
const DEFAULT_PORT = 8090

// Global Var
var database []CO.Product

// WaitGroup
var threadGroup sync.WaitGroup

// Mutex
var Mutex = &sync.Mutex{}

// Removes an item from the database based on the product ID
// the ... acts like a spread operator in javascript
// append takes a variable amount of arguments and ... allows use to the db slice as input
// if no ID found, nothing happens
func removeItem(db []CO.Product, id int) []CO.Product {
	for i, record := range(db) {
		if record.ID == id {
			db = append(db[:i], db[i+1:]...)
		} 
	}

	return db
}

// Does binary search to find the item in our DB list
// Items on the list are always in ascending order
func getItem(db *[]CO.Product, target int) (*CO.Product, error) {
	left, right := 0, len(*db)-1

	for left <= right {
		mid := (left+right)/2
		// return the address to the product object in the database
		if ((*db)[mid].ID == target) {
			return &(*db)[mid], nil
		} else if ((*db)[mid].ID < target) {
			left = mid+1
		} else {
			right = mid-1
		}
	}
	return &CO.Product{}, errors.New("ID not found")
}

// Returns the whole database to the client
func GetProductList(req CommandArgs, res *CommandResponse) error {
	Mutex.Lock()
	res.ProductList = database
	Mutex.Unlock()
	fmt.Println("returning a database to frontend")
	return nil
}

// Returns a Product to the client based on given Product ID
func GetProduct(req CommandArgs, res *CommandResponse) error {
	Mutex.Lock()
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	res.ProductItem = *item
	Mutex.Unlock()

	fmt.Println("return a product to frontend")
	return nil
}

// Edit Product Info based on given Product ID
func EditProduct(req CommandArgs, res *CommandResponse) error {
	Mutex.Lock()
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	item.Name = req.ProductInfo.Name
	item.Seller = req.ProductInfo.Seller
	item.Condition = req.ProductInfo.Condition
	item.Description = req.ProductInfo.Description
	item.Price = req.ProductInfo.Price

	Mutex.Unlock()

	fmt.Println("Editing Product ID: ", req.ProductID)
	return nil
}

// Creates a new product and append to database based on info given
func AddProduct(req CommandArgs, res *CommandResponse) error {
	price := req.ProductInfo.Price
	name := req.ProductInfo.Name
	description := req.ProductInfo.Description
	condition := req.ProductInfo.Condition
	seller := req.ProductInfo.Seller

	Mutex.Lock()
	database = append(database, CO.NewProduct(price, name, description, condition, seller))
	Mutex.Unlock()
	return nil
}

// Delete a product from database based on ProductID
func DeleteProduct(req CommandArgs, res *CommandResponse) error {

	Mutex.Lock()
	// Check if ID is valid
	_, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	database = removeItem(database, req.ProductID)
	Mutex.Unlock()
	return nil
}

// Backend Flag
type arrayFlag []string

func (i *arrayFlag) String() string {
	return ""
}

func (i *arrayFlag) Set(value string) error {
	*i = append(*i, strings.Split(value, ",")...)
	return nil
}

// Validates and transforms ip address if needed
func transformAddr (addr string) (string, bool) {
	backendAddrSplit := strings.Split(addr, ":")

	// With IP but no port indicated.
	if (len(backendAddrSplit) == 2 && len(backendAddrSplit[1]) == 0) {
		return "", false
	}

	// Address Validation to backend
	// Just assume the format is wrong if there are more than one colon
	if (strings.LastIndex(addr, ":") == -1 || len(backendAddrSplit) > 2) {
		return "", false
	} else if (strings.LastIndex(addr, ":") == 0) { // This is if user only enters port num
		if (len(addr) == 1) { // "only : case"
			addr = DEFAULT_BACKEND_ADDR
		} else {
			addr = "localhost" + addr
		}
	}

	return addr, true
}

func main() {
	// Raft Instance
	raft := new(Raft)

	startSignal = make(chan bool,1)
	startElection = make(chan bool, 1)
	convertFollower = make(chan bool, 1)
	heartbeatSignal = make(chan bool, 1)
	killHeart = make(chan bool,1 )
	startSignal <- true

	// Add some initial items into our db
	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))

	// Register a handler
	rpc.Register(raft)

	// Start up thread to listen on heartbeat port
	//go heartBeat("tcp")

	// Defines flags
	// isFirstLeader is needed since all the logs initially are all empty and the same
	// so no one would win the election. this helps us make one log different
	var portNum int
	var backendReplicas arrayFlag
	var nodeID string 

	flag.IntVar(&portNum, "listen", 8090, "This is the port number the application will listen on")
	flag.Var(&backendReplicas, "backend", "This is a list of all the other replicas on the backend")
	flag.StringVar(&nodeID, "id", "", "This is the public IP address for this node")

	// Parases the command line for flags
	flag.Parse()
	fmt.Println("Running on Port: ", portNum)

	nodeID, valid := transformAddr(nodeID)
	if (valid == false) {
		fmt.Println("id_flag: Wrong address format")
		fmt.Println("id_flag: Valid address format: <ip>:<port>")
		fmt.Println("id_flag: ", nodeID)
		os.Exit(1)
	}

	fmt.Println("Node ID: ", nodeID)
	fmt.Println("Raw replica addrs: ", backendReplicas)
	// Checks all the IP addresses to the backend replicas
	for i := 0; i < len(backendReplicas); i++ {
		fmt.Println(backendReplicas[i])
		formattedAddr, valid := transformAddr(backendReplicas[i])

		if (valid == false) {
			fmt.Println("backend_flag: Wrong address format")
			fmt.Println("backend_flag: Valid address format: <ip>:<port>")
			fmt.Println("backend_flag: ", backendReplicas[i])
			os.Exit(1)
		}

		backendReplicas[i] = formattedAddr
	}

	fmt.Println("Replica addrs: ", backendReplicas)

	servicePort := fmt.Sprintf(":%d", portNum)

	// Listen on the servicePort
	listener, err := net.Listen("tcp", servicePort)

	if (err != nil){
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Server now listening on", servicePort)
	// Create multi threads to listen to requests
	threadGroup.Add(NUM_THREADS)

	// Run the Raft instance
	go raft.runRaft(nodeID, backendReplicas)

	// Run all the RPC threads to listen for RPC requests
	for i := 0; i < NUM_THREADS; i++ {
		go rpc.Accept(listener)
	}

	// Have main thread wait forever until we force backend to quit
	threadGroup.Wait()
}