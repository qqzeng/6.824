#!/bin/sh

# Usage: ./test2A.sh 100 outputFile

if test $# -lt 2     # Insufficient number of parameters. 
then echo "Error! Usage：./test2A.sh testRoundNumber outputFile"
exit 1
fi

i=0
runTest2A=$(go test -run 2A)

echo "Begin run script "$0""
while [ $i -lt $1 ]
do
	echo $runTest2A >> $2
	i=`expr $i + 1`
done
echo "End run script "$0""
