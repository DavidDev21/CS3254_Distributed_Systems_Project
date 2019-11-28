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

/*
	Client with leader interaction
		The client should interact with leader through a specific RPC call
		that takes in what command they would like to execute along with the arguments and response structs like normal
		
		the leader should simply (after it proccess all the RAFT protocol normal operations and is ready to apply and commit the operation)
		pass the arguments along to the already existing RPC functions from phase 2
		Which should effectively pass back the results accordingly
*/

/*
	Crucial thoughts

	1.) Leader needs to keep track of all the log indices for each follower which is the purpose of nextIndex[]
			This allows the leader to be aware of what the followers need in their logs
			and also allows leader to know when it is safe to accept client requests again. (when we have a quorum of nodes that is fully synced up)
			// basically the nextIndex log matches up with what we have as the leader
	2.) AppendEntries should be sending one entry at a time based on whatever the follower is missing. as indicated by the nextIndex[i], i being the follower
			For each failed AppendEntries call to the follower, we decrement the nextIndex[i] which is a indicator the follower is missing entries or doesn't match
			We keep decrement until we get a successful AppendEntries indicating that we reached a point in the follower's log where it matches what we have

			The replication strategy from here is to increment on successful of each AppendEntries call and basically incrementally build up the 
			log of the follower
			(in the worst case, we rebuild the whole log with 2n messages, (n being the length of the log))

	3.) Client Leader interaction
			have an RPC method the client will be able to use to talk to the backend
			-- The RPC method should take in what command client wants and the arguments
			-- It should check what state we are in, If we are not Leader, then reply back with the Leaders ID (nodeID)
			-- If we are the leader, then we send off the given arguments from client to a separate thread (the RAFT thread) for processing
					-- Results should be handled or passed through a one capacity channel
					-- This way the RPC method will know when to send back the results to the client
		RAFT must run on a separate thread because of:
			1.) the built in heartbeat mechanic of RAFT, which needs to keep going regardless of client interaction
			2.) All backend replicas must run its own logic independent of the client
		This also forces the client to wait for us to finish, and also allows us to process multiple clients requests in a relatively safe matter
		-- The results channel can be limited to size 1 so we can process one client at a time, while acknowledging multiple clients at once
		-- Effectively the concurrent client requests will be processed based on the non-deterministic behavior

	4.) Timeout mechancism
			Ideally, have a separate thread only for timeout
			Where there is a timeout, we look at our current state, and then take steps to transition to a different state
			Ex:
				at the moment of the timeout, 
				a.) if we are still the candidate, rerun the candidate protocol
				b.) if we are in the follower state, then change state to candidate, and run candidate protocol
				c.) if we are in the leader state, we shouldn't be timing out as a leader

			Check playground for a possible skeleton that works
	
	5.) Running the states as go routines, Seems to cause more trouble than benefit
		- The unpredictable behavior of the go routines and the need to loop constantly within a state causes unpredictable
		behavior that is hard to debug
		- We need a separate channels to cause the main raft thread to quit or change states
		- we can safely process RPC calls on a separate thread but the main raft instance must run on the main thread for safety.
*/


// Raft Stuff
const FOLLOWER_STATE = "follower"
const LEADER_STATE = "leader"
const CANDIDATE_STATE = "candidate"
const HEARTBEAT_PERIOD = time.Duration(50) * time.Millisecond // Should be less than the min timeout time
const TIMEOUT_RANGE_START = 150
const TIMEOUT_RANGE_END = 500

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
var timerMutex = &sync.Mutex{}

// Sets the timer for random time between the start and end range
func SetTimer(start, end int) {
	timeout = rand.Intn(end) + start
	timer = time.After(time.Duration(timeout) * time.Millisecond)
}

// We need the arguments that come with the commands
// In order to replicate the state of the database
type CommandArgs struct {

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
	committedIndex int // the highest Log Entry that was known to be committed, i.e, to have been executed by leader
	lastApplied int // the highest Log Entry that was actually executed on our local FSM (the database)
	log []LogEntry // replica log for FSM, holds the termNumber and command from leader

	// In the event of a crash, (log not in disk)
	// Might just be easier to grab the whole log from the current leader
	nextIndex []int // track of what the next log entry should be sent to i'th follower
	// matchIndex would be for replication efficiency in the event the follower is significantly behind the leader
	matchIndex []int // track of the last index where each follower matches with the leader's log
	state string

	quorumSize int // the minimum quorum we need given our initial configuration
	leaderID string
	replicas []string
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

	// Reset Timer now that we got an msg from leader
	heartbeatSignal <- true // only to be picked up by the follower
	// Once we get an AppendEntries from the new leader, forget who we votedFor during the election
	self.votedFor = ""

	fmt.Println("Raft.AppendEntries: Received from ", req.LeaderID)

	firstCondition := self.state == CANDIDATE_STATE
	secondCondition := (self.state == LEADER_STATE && self.currentTerm < req.Term)

	fmt.Println("AppendEntries First Condition: ", firstCondition)
	fmt.Println("AppendEntries Second Condition: ", secondCondition)

	// We are no longer the leader if we were the leader, and also can't be a candidate if a leader was elected
	if (firstCondition || secondCondition) {
		self.state = FOLLOWER_STATE
		
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

// Work In Progress: current skelelton
// self: here should be the receiver end meaning a follower
// Args: term int, candidateID string, lastLogIndex int, lastLogTerm int
func (self *Raft) RequestVote(req RequestVoteArgs, res *RequestVoteResponse) error {
	// Note: the only time we would ever vote for someone is if our term is less than candidate term
	// If candidate claims the same term as us, then we can believe that we already voted for someone in the current term
	// (we would only be the same if we had voted, since we are forced to update our current term to matched the candidate that we are voting for)

	// Large Critical Section is more worth since all the operations are relatively short
	RaftMutex.Lock()
	defer RaftMutex.Unlock()

	fmt.Println("In RequestVote: Connection from ", req.CandidateID)
	fmt.Println("Candidate Term: ", req.Term, self.currentTerm)

	// Case: we already voted for someone else in the current term / the candidate's term is behind (covers 2 cases)
	if (self.votedFor != req.CandidateID && req.Term <= self.currentTerm) {
		res.Term = self.currentTerm
		res.VoteGranted = false

		//return self.currentTerm, false
	}

	// Case: we already voted for the candidate that is requesting for the current term(in the event of network failure)
	if (self.votedFor == req.CandidateID && self.currentTerm == req.Term) {
		res.Term = self.currentTerm
		res.VoteGranted = true
		//return self.currentTerm, true
	}

	// First Check if Caller's term is not behind ours
	// If so, we don't give them our vote since they are not eligible for leadership
	// if (term < self.currentTerm) {
	// 	return self.currentTerm, false
	// }

	// We grant the vote if we haven't voted for anyone, or if we already voted for candidateID.
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

	fmt.Println("Raft.RequestVote First Condition: ", firstCondition)
	fmt.Println("Raft.RequestVote Second Condition: ", secondCondition)
	if (firstCondition || secondCondition) {
		self.votedFor = req.CandidateID
		self.currentTerm = req.Term
		res.Term = self.currentTerm
		res.VoteGranted = true
		return nil
	}

	/*
	if (req.lastLogTerm > self.log[currentLastIndex].termNumber) {
		self.votedFor = req.candidateID
		self.currentTerm = req.term
		res.term = self.currentTerm
		res.voteGranted = true
		return nil
		//return self.currentTerm, true
	}

	if (req.lastLogTerm == self.log[currentLastIndex].termNumber && req.lastLogIndex > currentLastIndex) {
		self.votedFor = req.candidateID
		self.currentTerm = req.term
		res.term = self.currentTerm
		res.voteGranted = true
		return nil
		//return self.currentTerm, true
	}
	*/

	res.Term = self.currentTerm
	res.VoteGranted = false

	return nil
	//return self.currentTerm, false
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
	self.lastApplied = 0
	self.replicas = replicas
	self.quorumSize = (len(replicas)/2) + 1

	self.log = []LogEntry{LogEntry{TermNumber: 0, Command: ""}}
}

// Algo for each state
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

func (self *Raft) leaderHeartbeat(req *AppendEntriesArgs, replicaAddr string) {

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
	}
	return
}

// This is a separate thread for the leader to have heartbeats in regular intervals
// Also allows us to control the frequency of these messages
func (self *Raft) heartbeatController(req *AppendEntriesArgs) {

	for {
		select {
		case <- killHeart:
			// Leader told us to kill the heart
			return
		default:
			// Otherwise we keep beating
			for _, replicaAddr := range self.replicas {
				go self.leaderHeartbeat(req, replicaAddr)
			}
			time.Sleep(HEARTBEAT_PERIOD)
		}
	}
}

// Leader Functions
// The Leader methods should be separate into two components
// The normal heartbeat which composes of empty AppendEntries
// and the client request processing, which should happen whenver the client requests
// This is to prevent a flood of heartbeat messages and allow for additional control flow
func (self *Raft) runLeader() {
	fmt.Println("Entering Leader")
	req := new(AppendEntriesArgs)
	// More to be added
	req.Term = self.currentTerm
	req.LeaderID = self.ID

	// Starts the heartbeat of the leader
	go self.heartbeatController(req)

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

// Specific RPC call for client to call when they want to request
func (self *Raft) processClient() {

}

// Simply empties out anything in the channel
func cleanChannel(c chan bool) {
	lenChan := len(c)
	for i := 0; i < lenChan; i++ {
		<-c
	}
}

// Simply tries to get a vote from replicaAddr
// If it fails, then it just fails
// We can simply just try to get their vote again next term
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
			fmt.Println("GetVote: outdated term, converting to follower")
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
			fmt.Println("We got a vote from ", replicaAddr)
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

// Ideally: calls another RPC message on the receiver (follower)
// to get a vote from them
// GetVotes() should return the number of votes we got back from the other replicas
// We count those replicas that we can't reach as a no vote
// Self: here should be the candidate in this case, ourselves
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

	// We want to clean the channel in case of residual data from previous getVote threads
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
			fmt.Println("I won : ", self.leaderID )
			fmt.Println("Votes gotten back: ", numVotes)
			startSignal <- true // Signal the main thread to go to a different state
			RaftMutex.Unlock()
			return // Exit
		}

		// If we still need to retry a connection, then signal to do so
		for _, needToConnect := range connectionToMake {
			if (needToConnect == true) {
				startElection <- true
				fmt.Println(connectionToMake)
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

// Ideally running this in a separate thread (or basically the main thread)
// Separate from the RPCs
// Self: is the current Raft instance on this machine
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
//const HEARTBEAT_PORT = ":8091"

// Global Var
var database []CO.Product

// WaitGroup
var threadGroup sync.WaitGroup

// Mutex
var Mutex = &sync.Mutex{}

/*
// Simply accept a connection from whoever wants to know if the server is alive
func heartBeat(protocol string) {
	sock, err := net.Listen(protocol, HEARTBEAT_PORT)
	if (err != nil) {
		fmt.Println(err)
	}
	for {
		conn, err := sock.Accept()
		fmt.Println("Sending heartbeat")
		if (err != nil) {
			fmt.Println(err)
		}
		defer conn.Close()
	}
}
*/

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

/* RPC HANDLERS */
type DB struct {}
type Request struct {
	ProductID int
	ProductInfo CO.Product
}
type Response struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

// Returns the whole database to the client
func (t *DB) GetProductList(req Request, res *Response) error {
	Mutex.Lock()
	res.ProductList = database
	Mutex.Unlock()
	fmt.Println("returning a database to frontend")
	return nil
}

// Returns a Product to the client based on given Product ID
func (t *DB) GetProduct(req Request, res *Response) error {
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
func (t *DB) EditProduct(req Request, res *Response) error {
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
func (t *DB) AddProduct(req Request, res *Response) error {
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
func (t *DB) DeleteProduct(req Request, res *Response) error {

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
	rpc.Register(new(DB))
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

	go raft.runRaft(nodeID, backendReplicas)

	for i := 0; i < NUM_THREADS; i++ {
		go rpc.Accept(listener)
	}

	// Have main thread wait forever until we force backend to quit
	threadGroup.Wait()
}