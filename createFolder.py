#! /usr/bin/env python
import os

print("creating folder")

for i in range(100):
	path = "./images/" + str(i)
	if not os.path.exists(path):
		os.makedirs(path)

print("done")

