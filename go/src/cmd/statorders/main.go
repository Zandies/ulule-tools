package main

import (
	"fmt"
	"github.com/GeertJohan/go.linenoise"
	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"sort"
	"strconv"
	"strings"
	"ulule/clientapi"
	// "ulule/credentials"
	"github.com/tealeg/xlsx"
)

var ()

func main() {

	conn, err := redis.Dial("tcp", "127.0.0.1:6379")
	if err != nil {
		logrus.Fatal(err)
	}

	values, err := redis.Values(conn.Do("SMEMBERS", "syncs"))
	if err != nil {
		logrus.Fatal(err)
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if ok {
			fmt.Println(string(b))
		}
	}

	syncName, err := linenoise.Line("use sync> ")
	if err != nil {
		logrus.Fatal(err)
	}

	// display project summary
	values, err = redis.Values(conn.Do("HGETALL", syncName+"_project"))
	if err != nil {
		logrus.Fatal("HGETALL "+syncName+"_project error:", err)
	}
	stringMap, err := redis.StringMap(values, nil)
	if err != nil {
		logrus.Fatal("redis.StringMap err:", err)
	}
	// logrus.Printf("%+v", stringMap)

	fmt.Println("project:", stringMap["slug"])
	fmt.Println("rewards:")
	nbRewards, err := strconv.Atoi(stringMap["nbrewards"])
	if err != nil {
		logrus.Fatal(err)
	}

	for i := 0; i < nbRewards; i++ {
		index := strconv.Itoa(i)
		id := stringMap["reward"+index+"_id"]
		price := stringMap["reward"+index+"_price"]
		fmt.Println(id, price)
	}

	fmt.Println("commands: export [rewards], countries [rewards], exit")

	for {

		cmd, err := linenoise.Line("> ")
		if err != nil {
			logrus.Fatal(err)
		}

		parts := strings.Split(cmd, " ")
		if len(parts) > 0 {
			cmd = parts[0]
		}
		args := []string{}
		if len(parts) > 1 {
			args = parts[1:]
		}

		switch cmd {
		case "exit":
			break
		case "countries":
			displayCountries(syncName, conn, args)
		case "export":
			exportPaidOrders(syncName, conn, args)

		}
	}
}

func isValid(invalidOrderIDs []string, orderID string) bool {
	for _, invalidOrderID := range invalidOrderIDs {
		if orderID == invalidOrderID {
			return false
		}
	}
	return true
}

func exportPaidOrders(syncName string, conn redis.Conn, rewardIDs []string) {
	values, err := redis.Values(conn.Do("SMEMBERS", syncName))
	if err != nil {
		logrus.Fatal(err)
	}

	var file *xlsx.File
	var sheet *xlsx.Sheet

	var row *xlsx.Row
	var cell *xlsx.Cell

	file = xlsx.NewFile()
	sheet, err = file.AddSheet("Sheet1")
	if err != nil {
		logrus.Fatal(err)
	}

	// get invalid orders, to exclude them form export
	invalidOrders, errInvalidOrders := redis.Strings(redis.Values(conn.Do("SMEMBERS", syncName+"_invalidOrders")))
	if errInvalidOrders != nil {
		logrus.Fatal(errInvalidOrders)
	}
	// logrus.Println("invalidOrders:", invalidOrders)

	conn.Send("MULTI")

	// tmp
	nbEntries := 0
	nbPaymentInvalid := 0
	nbInvalid := 0

	for _, value := range values {
		b, ok := value.([]byte)
		if ok {
			orderID := string(b)
			conn.Send("HGETALL", syncName+"_order_"+orderID)
		}
	}

	values, err = redis.Values(conn.Do("EXEC"))
	if err != nil {
		logrus.Fatal(err)
	}

	for _, value := range values {
		stringMap, _ := redis.StringMap(value, nil)

		nbItems, _ := strconv.Atoi(stringMap["nbItems"])
		if nbItems == 0 {
			continue
		}

		accept := true
		// filter reward ids
		if rewardIDs != nil && len(rewardIDs) > 0 {
			accept = false
			// HACK: considering there's only one
			// single item per order (id: 0)
			rewardID := stringMap["item0_product"]
			//logrus.Println(rewardID)

			for _, id := range rewardIDs {
				if id == rewardID {
					accept = true
					break
				}
			}
		}

		if accept {
			paymentStatus, _ := strconv.Atoi(stringMap["status"])
			accept = clientapi.OrderStatus(paymentStatus) == clientapi.OrderStatusPaymentDone
			if !accept && clientapi.OrderStatus(paymentStatus) == clientapi.OrderStatusInvalid {
				nbPaymentInvalid++
			}
		}

		if accept {
			// invalid orders are the one that can't be sent because users made a mistake
			// in the shipping address (lefting one or more fields blank)
			accept = isValid(invalidOrders, stringMap["orderId"])
			if !accept {
				nbInvalid++
			}
		}

		if accept {

			//logrus.Println(stringMap["statusDisplay"])
			// logrus.Println(stringMap)

			// logrus.Println(stringMap["firstName"]+" "+stringMap["lastName"], "|",
			// 	stringMap["shippingAddr1"], "|",
			// 	stringMap["shippingAddr2"], "|",
			// 	stringMap["shippingCity"], "|",
			// 	stringMap["shippingCode"], "|",
			// 	stringMap["shippingCountry"], "|",
			// 	stringMap["email"], "|")

			row = sheet.AddRow()
			cell = row.AddCell()
			cell.Value = stringMap["firstName"] + " " + stringMap["lastName"]

			cell = row.AddCell()
			cell.Value = stringMap["shippingAddr1"]

			cell = row.AddCell()
			cell.Value = stringMap["shippingAddr2"]

			cell = row.AddCell()
			cell.Value = stringMap["shippingCity"]

			// state
			cell = row.AddCell()

			cell = row.AddCell()
			cell.Value = stringMap["shippingCode"]

			// country name
			cell = row.AddCell()

			cell = row.AddCell()
			cell.Value = stringMap["shippingCountry"]

			// phone number
			cell = row.AddCell()

			cell = row.AddCell()
			cell.Value = stringMap["email"]

			nbEntries++
			// if nbDisplayed >= 10 {
			// 	break
			// }
		}
	}

	fileName := "/data/" + syncName + ".xlsx"
	err = file.Save(fileName)
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Println(nbEntries, "entries written in", fileName)
	logrus.Println("(" + strconv.Itoa(nbPaymentInvalid) + " payment invalid)")
	logrus.Println("(" + strconv.Itoa(nbInvalid) + " invalid shipping addresses)")
}

func displayCountries(syncName string, conn redis.Conn, rewardIDs []string) {
	values, err := redis.Values(conn.Do("SMEMBERS", syncName))
	if err != nil {
		logrus.Fatal(err)
	}

	conn.Send("MULTI")

	for _, value := range values {
		b, ok := value.([]byte)
		if ok {
			orderID := string(b)
			conn.Send("HGETALL", syncName+"_order_"+orderID)
		}
	}

	values, err = redis.Values(conn.Do("EXEC"))
	if err != nil {
		logrus.Fatal(err)
	}

	countries := make(map[string]int)
	nbContributions := 0

	for _, value := range values {
		stringMap, _ := redis.StringMap(value, nil)

		nbItems, _ := strconv.Atoi(stringMap["nbItems"])
		if nbItems == 0 {
			continue
		}

		accept := true
		// filter reward ids
		if rewardIDs != nil && len(rewardIDs) > 0 {
			accept = false
			// HACK: considering there's only one
			// single item per order (id: 0)
			rewardID := stringMap["item0_product"]
			//logrus.Println(rewardID)

			// if rewardID == "177453" {
			// 	logrus.Printf("%#v", stringMap)
			// }

			for _, id := range rewardIDs {
				if id == rewardID {
					accept = true
					break
				}
			}
		}

		if accept {
			paymentStatus, _ := strconv.Atoi(stringMap["status"])
			accept = clientapi.OrderStatus(paymentStatus) == clientapi.OrderStatusPaymentDone ||
				clientapi.OrderStatus(paymentStatus) == clientapi.OrderStatusInvalid
		}

		if accept {
			//logrus.Println(stringMap["statusDisplay"])

			if _, exist := countries[stringMap["shippingCountry"]]; !exist {
				countries[stringMap["shippingCountry"]] = 1
			} else {
				countries[stringMap["shippingCountry"]]++
			}

			nbContributions++
		}
	}

	pl := make(PairList, len(countries))
	i := 0

	for country, count := range countries {
		if country == "" {
			country = "none"
		}
		// fmt.Println(country, ":", count)
		pl[i] = Pair{country, count}
		i++
	}

	sort.Sort(sort.Reverse(pl))

	for _, pair := range pl {
		fmt.Println(pair.Key, ":", pair.Value)
	}
	fmt.Println("contributions:", nbContributions)
}

type Pair struct {
	Key   string
	Value int
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
