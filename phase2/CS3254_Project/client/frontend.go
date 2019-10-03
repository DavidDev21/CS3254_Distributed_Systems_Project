/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 1
	main.go will serve as the starting point for the web app
*/

package main

import ("fmt"
		"github.com/kataras/iris"
		"flag"
		"os"
		"net/rpc"
		"strings"
		CO "CS3254_Project/custom_objects"
		)

/* RPC interfaces */
type DB struct {}
type Request struct {
	ProductID int
	ProductInfo CO.Product
}
type Response struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

// Performs RPC Dial to address under the indicated network protocol
// Exits the program if there was an error
func DialWithCheck(protcol, address string) (*rpc.Client) {
	conn, err := rpc.Dial(protcol, address)

	if (err != nil) {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	return conn
}

const DEFAULT_BACKEND_ADDR = "localhost:8090"
const DEFAULT_PORT = 8080

func main() {
	// Defines the listen, backend flag
	var portNum int
	var backendAddr string

	flag.IntVar(&portNum, "listen", DEFAULT_PORT, "this is the port number the application will listen on")
	flag.StringVar(&backendAddr, "backend", DEFAULT_BACKEND_ADDR, "this is the address to the backend. <ip>:<port>")
	
	// Parses the command line for flags
	flag.Parse()
	fmt.Println("Frontend Port:", portNum)

	// Address Validation to backend
	// Just assume the format is wrong if there are more than one colon
	if (strings.LastIndex(backendAddr, ":") == -1 || len(strings.Split(backendAddr, ":")) > 2) {
		fmt.Println("backend_flag: Wrong address format")
		fmt.Println("backend_flag: Valid address format: <ip>:<port>")
		os.Exit(1)
	} else if (strings.LastIndex(backendAddr, ":") == 0) { // This is if user only enters port num
		if (len(backendAddr) == 1) { // "only : case"
			backendAddr = DEFAULT_BACKEND_ADDR
		} else {
			backendAddr = "localhost" + backendAddr
		}
	}

	fmt.Println("Backend Addr:" , backendAddr)

	// IRIS setup
	app := iris.Default()
	serverAddr := fmt.Sprintf(":%d", portNum)

	// View template registeration
	app.RegisterView(iris.HTML("../views", ".html"))

	// Tells IRIS where is the styles folder
	app.HandleDir("/styles", "../views/styles")

	// GET method for route "/"
	app.Handle("GET", "/", func(ctx iris.Context) {
		// Connect to backend
		conn := DialWithCheck("tcp", backendAddr)

		defer conn.Close()

		req, res := new(Request), new(Response)

		// RPC Call
		conn.Call("DB.GetProductList", req, res)

		// Render page
		ctx.ViewData("database", res.ProductList)
		ctx.View("index.html")
	})

	// GET edit product page
	app.Handle("GET", "/edit/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn := DialWithCheck("tcp", backendAddr)

			defer conn.Close()

			req, res := &Request{ProductID: id}, new(Response)

			// RPC Call
			err = conn.Call("DB.GetProduct", req, res)

			if (err != nil) {
				fmt.Println(err)
			}

			// Render page
			ctx.ViewData("product", res.ProductItem)
			ctx.View("editProduct.html")
		}
	})

	// POST edit product page
	// Handles the update of a product
	app.Handle("POST", "/edit/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameters
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn := DialWithCheck("tcp", backendAddr)
			
			defer conn.Close()

			req, res := new(Request), new(Response)

			// Args for RPC
			req.ProductID = id
			req.ProductInfo.Name = ctx.PostValue("name")
			req.ProductInfo.Seller = ctx.PostValue("seller")
			req.ProductInfo.Condition = ctx.PostValue("condition")
			req.ProductInfo.Description = ctx.PostValue("description")
			req.ProductInfo.Price, _ = ctx.PostValueInt("price")

			// RPC call
			err = conn.Call("DB.EditProduct", req, res)

			if (err != nil) {
				fmt.Println(err)
			}
			
			// return home
			ctx.Redirect("/", 302)
		}
	})

	// GET Add product
	app.Handle("GET", "/createProduct", func(ctx iris.Context) {
		ctx.View("createProduct.html")
	})
 
	// POST Add product
	app.Handle("POST", "/createProduct", func(ctx iris.Context) {
		// Connect to backend
		conn := DialWithCheck("tcp", backendAddr)

		defer conn.Close()

		req, res := new(Request), new(Response)

		// Args for RPC
		req.ProductInfo.Name = ctx.PostValue("name")
		req.ProductInfo.Seller = ctx.PostValue("seller")
		req.ProductInfo.Condition = ctx.PostValue("condition")
		req.ProductInfo.Description = ctx.PostValue("description")
		req.ProductInfo.Price, _ = ctx.PostValueInt("price")

		// RPC Call
		err := conn.Call("DB.AddProduct", req, res)

		if (err != nil) {
			fmt.Println(err)
		}

		// return home
		ctx.Redirect("/", 302)
	})

	// POST Delete product
	app.Handle("POST", "/delete/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn := DialWithCheck("tcp", backendAddr)

			defer conn.Close()

			req, res := new(Request), new(Response)

			// Args for RPC
			req.ProductID = id

			// RPC Call
			err = conn.Call("DB.DeleteProduct", req, res)

			if (err != nil) {
				fmt.Println(err)
			}

			// return home
			ctx.Redirect("/", 302)
		}
	})

	// IRIS listening on port
	app.Run(iris.Addr(serverAddr), iris.WithoutServerError(iris.ErrServerClosed))
}
