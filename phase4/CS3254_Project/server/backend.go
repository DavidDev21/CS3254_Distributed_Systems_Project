/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 2 (Backend)
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
		CO "CS3254_Project/custom_objects")

// Constants
const NUM_THREADS = 5
const DEFAULT_BACKEND_ADDR = "localhost:8090"
const DEFAULT_PORT = 8090
const HEARTBEAT_PORT = ":8091"

// Global Var
var database []CO.Product

// WaitGroup
var threadGroup sync.WaitGroup

// Mutex
var Mutex = &sync.Mutex{}

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
	// Add some initial items into our db
	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))

	// Register a handler
	rpc.Register(new(DB))

	// Start up thread to listen on heartbeat port
	go heartBeat("tcp")

	// Defines the listen flag
	var portNum int
	var backendReplicas arrayFlag
	flag.IntVar(&portNum, "listen", 8090, "this is the port number the application will listen on")

	flag.Var(&backendReplicas, "backend", "This is a list of all the other replicas on the backend")
	// Parases the command line for flags
	flag.Parse()
	fmt.Println(portNum)

	fmt.Println(backendReplicas)
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

	fmt.Println(backendReplicas)

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

	for i := 0; i < NUM_THREADS; i++ {
		go rpc.Accept(listener)
	}

	// Have main thread wait forever until we force backend to quit
	threadGroup.Wait()
}