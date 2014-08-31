package wigo

// Events

const (
	ADDDIRECTORY    	= 1
	REMOVEDIRECTORY 	= 2

	NEWPROBERESULT    	= 3
	DELETEPROBERESULT 	= 4

	NEWREMOTERESULT		= 5
	NEWCONNECTION     	= 6

	SENDRESULTS 		= 7
	SENDNOTIFICATION	= 8
)

type Event struct {
	Type  int
	Value interface{}
}
