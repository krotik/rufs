#!/bin/bash

if [ -z "$1" ]
  then
    set -- "rufs"
fi

# Use this simple script to attach the web.zip to Rufs's executable.

for f in "$@"
do
  if grep -Fxq "####WEBZIP####" $f
  then
    echo "File web.zip is already appended to $f"
  else
    echo "Appending web.zip to $f ..."
    echo >> $f
    echo "####WEBZIP####" >> $f
    cat web.zip >> $f
  fi
done

