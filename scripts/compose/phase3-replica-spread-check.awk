BEGIN {
  FS = "|"
  required = "gateway frontend backend"
}

$1 != "headers" {
  values[$1] = $2 + 0
  seen[$1] = 1
}

function fail(message) {
  print message > "/dev/stderr"
  status = 1
}

END {
  count = split(required, keys, " ")
  for (i = 1; i <= count; i++) {
    tier = keys[i]
    if (!seen[tier]) {
      fail("missing " tier " replica spread evidence")
    } else if (values[tier] < 3) {
      fail("expected at least 3 " tier " replicas, got " values[tier])
    }
  }
  exit status
}
