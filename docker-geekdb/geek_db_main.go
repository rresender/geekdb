package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/gorilla/mux"

	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
)

// MaxMembers : Max Number of members to notify
const MaxMembers = 2

func main() {

	addr := os.Getenv("HOST_ADDR")
	cluster, err := setupCluster(addr, os.Getenv("CLUSTER_ADDR"))

	if err != nil {
		log.Fatal(err)
	}
	defer cluster.Leave()

	entry := InitEntry(11) // Initial value
	launchHTTP(entry)

	ctx := context.Background()
	ctx = context.WithValue(ctx, nameKey, addr)

	log.Printf("Starting Server...\n")

	debugDataPrinterTicker := time.Tick(5 * time.Second)
	numberBroadcastTicker := time.Tick(2 * time.Second)

	for {
		select {
		case <-numberBroadcastTicker:
			members := getOtherMembers(cluster)
			ctx, cancel := context.WithTimeout(ctx, time.Second*2)
			defer cancel()
			go notifyOthers(ctx, members, entry)
		case <-debugDataPrinterTicker:
			log.Printf("Members: %v\n", cluster.Members())
			curVal, curGen := entry.getValue()
			log.Printf("State: Val: %v Gen: %v\n", curVal, curGen)
		}
	}
}

func setupCluster(advertiseAddr string, clusterAddr string) (*serf.Serf, error) {

	conf := serf.DefaultConfig()
	conf.Init()
	conf.MemberlistConfig.AdvertiseAddr = advertiseAddr

	cluster, err := serf.Create(conf)

	if err != nil {
		return nil, errors.Wrap(err, "Couldn't create cluster")
	}

	_, err = cluster.Join([]string{clusterAddr}, true)
	if err != nil {
		log.Printf("Couldn't join cluster, starting own: %v\n", err)
	}
	return cluster, nil
}

func launchHTTP(db *Entry) {
	go func() {
		m := mux.NewRouter()
		m.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
			val, _ := db.getValue()
			fmt.Fprintf(w, "%v", val)
		})

		m.HandleFunc("/set/{newVal}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			newVal, err := strconv.Atoi(vars["newVal"])
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "%v", err)
				return
			}

			db.setValue(newVal)

			fmt.Fprintf(w, "%v", newVal)
		})

		m.HandleFunc("/notify/{curVal}/{curGeneration}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			curVal, err := strconv.Atoi(vars["curVal"])
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "%v", err)
				return
			}

			curGeneration, err := strconv.Atoi(vars["curGeneration"])
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "%v", err)
				return
			}

			if changed := db.notifyValue(curVal, curGeneration); changed {
				log.Printf(
					"NewVal: %v Gen: %v Notifier: %v",
					curVal,
					curGeneration,
					r.URL.Query().Get("notifier"))
			}
			w.WriteHeader(http.StatusOK)
		})
		log.Fatal(http.ListenAndServe(":8080", m))
	}()
}

func getOtherMembers(cluster *serf.Serf) []serf.Member {
	members := cluster.Members()
	for i := 0; i < len(members); {
		if members[i].Name == cluster.LocalMember().Name || members[i].Status != serf.StatusAlive {
			if i < len(members)-1 {
				members = append(members[:i], members[i+1:]...)
			} else {
				members = members[:i]
			}
		} else {
			i++
		}
	}
	return members
}

func notifyOthers(ctx context.Context, otherMembers []serf.Member, db *Entry) {

	g, ctx := errgroup.WithContext(ctx)

	if len(otherMembers) <= MaxMembers {
		for _, member := range otherMembers {
			curMember := member
			g.Go(func() error {
				return notifyMember(ctx, curMember.Addr.String(), db)
			})
		}
	} else {
		randIndex := rand.Int() % len(otherMembers)
		for i := 0; i < MaxMembers; i++ {
			g.Go(func() error {
				return notifyMember(
					ctx,
					otherMembers[(randIndex+i)%len(otherMembers)].Addr.String(), db)
			})
		}
	}

	err := g.Wait()
	if err != nil {
		log.Printf("Error when notifying other members: %v", err)
	}
}

func notifyMember(ctx context.Context, addr string, db *Entry) error {
	val, gen := db.getValue()

	url := fmt.Sprintf("http://%v:8080/notify/%v/%v?notifier=%v", addr, val, gen, ctx.Value(nameKey))

	log.Printf("Url to Notify: %v", url)

	req, err := http.NewRequest("POST", url, nil)

	if err != nil {
		return errors.Wrap(err, "Couldn't create request")
	}

	req = req.WithContext(ctx)

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Couldn't make request")
	}
	return nil
}
