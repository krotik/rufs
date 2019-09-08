#!/bin/sh
cd "$(dirname "$0")"

if ! [ -d "run" ]; then
  mkdir -p run
  cp -R res/* run
fi

cd run
../../../rufs server
