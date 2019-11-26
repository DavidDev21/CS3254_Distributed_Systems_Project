## CS3254 Project

# Phase 1
'''
Create a basic web app with golang IRIS
'''

# Phase 2
'''
Separate original app into frontend and backend server
'''

Leader
'''
go run backend.go --listen=8090 --id=:8090 --backend=:8091,:8092 --leader=true
'''

Others
'''
go run backend.go --listen=8091 --id=:8091 --backend=:8090,:8092
'''