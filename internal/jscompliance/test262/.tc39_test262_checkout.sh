#!/bin/sh -e
sha=3af36bec45bd4f72d4b57366653578e1e4dafef7
mkdir -p testdata/test262
cd testdata/test262
if [ ! -d .git ]; then
  git init
  git remote add origin https://github.com/tc39/test262.git
fi
git fetch origin --depth=1 "${sha}"
git reset --hard "${sha}"
cd -
