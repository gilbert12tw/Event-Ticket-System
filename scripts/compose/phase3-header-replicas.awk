BEGIN {
  FS = ": *"
  want = tolower(header)
}

tolower($1) == want {
  value = $2
  sub(/\r$/, "", value)
  if (value != "") seen[value] = 1
}

END {
  count = 0
  for (value in seen) count++
  print count
}
