package mapreduce

import (
	"encoding/json"
	"log"
	"os"
	"sort"
)

func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTask int, // which reduce task this is
	outFile string, // write the output here
	nMap int, // the number of map tasks that were run ("M" in the paper)
	reduceF func(key string, values []string) string,
) {
	//
	// doReduce manages one reduce task: it should read the intermediate
	// files for the task, sort the intermediate key/value pairs by key,
	// call the user-defined reduce function (reduceF) for each key, and
	// write reduceF's output to disk.
	//
	// You'll need to read one intermediate file from each map task;
	// reduceName(jobName, m, reduceTask) yields the file
	// name from map task m.
	//
	// Your doMap() encoded the key/value pairs in the intermediate
	// files, so you will need to decode them. If you used JSON, you can
	// read and decode by creating a decoder and repeatedly calling
	// .Decode(&kv) on it until it returns an error.
	//
	// You may find the first example in the golang sort package
	// documentation useful.
	//
	// reduceF() is the application's reduce function. You should
	// call it once per distinct key, with a slice of all the values
	// for that key. reduceF() returns the reduced value for that key.
	//
	// You should write the reduce output as JSON encoded KeyValue
	// objects to the file named outFile. We require you to use JSON
	// because that is what the merger than combines the output
	// from all the reduce tasks expects. There is nothing special about
	// JSON -- it is just the marshalling format we chose to use. Your
	// output code will look something like this:
	//
	// enc := json.NewEncoder(file)
	// for key := ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()
	//
	// Your code here (Part I).
	//

	// 1. Read all intermidate files one by one from map task.
	kvs := make(map[string][]string)
	for m := 0; m < nMap; m++ {
		name := reduceName(jobName, m, reduceTask)
		log.Println("DoReduce - Load intermidate file :", name)
		file, err := os.Open(name)
		if err != nil {
			log.Fatal("DoReduce - Load intermidate file error : ", err)
		}

		// 2. Read content of each file by decoding it with JSON
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			err = dec.Decode(&kv)
			if err != nil {
				break
			}
			// 3. Store kv to the map, create a new entray array for the corresponding key if it does not exist.
			_, ok := kvs[kv.Key]
			if !ok {
				var tmp []string
				kvs[kv.Key] = tmp
				// kvs[kv.Key] = make([]string, 1) // Not right. it will cause a extral preceding zero value in array.
			}
			kvs[kv.Key] = append(kvs[kv.Key], kv.Value)
		}
		file.Close()
	}

	// 4. Sort the map according to the key.
	var keys []string
	for k, _ := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 6. Write results of ReduceF to disk by encoding them with JSON.
	part := mergeName(jobName, reduceTask)
	file, err := os.Create(part)
	if err != nil {
		log.Fatal("DoReduce - Create result file error : ", err)
	}
	enc := json.NewEncoder(file)
	// 5. Call ReduceF to get results.
	for _, k := range keys {
		res := reduceF(k, kvs[k])
		enc.Encode(KeyValue{k, res})
	}
	file.Close()
}
