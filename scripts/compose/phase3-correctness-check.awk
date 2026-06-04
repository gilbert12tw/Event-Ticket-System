BEGIN {
  FS = "|"
  status = 0
  required = "matching_events capacity confirmed tickets confirmed_missing_ticket confirmed_inactive_ticket duplicate_active_registrations duplicate_checkins booking_confirmed_audits ticket_issued_audits"
}

{
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
    if (!seen[keys[i]]) {
      fail("missing post-load correctness field: " keys[i])
    }
  }

  if (values["matching_events"] != 1) {
    fail("expected exactly one benchmark event; observed " values["matching_events"])
  }
  if (values["confirmed"] > values["capacity"]) {
    fail("confirmed bookings exceeded event capacity: confirmed=" values["confirmed"] " capacity=" values["capacity"])
  }
  if (values["tickets"] != values["confirmed"]) {
    fail("ticket count does not match confirmed registrations: tickets=" values["tickets"] " confirmed=" values["confirmed"])
  }
  if (values["confirmed_missing_ticket"] != 0) {
    fail("confirmed registrations missing tickets: " values["confirmed_missing_ticket"])
  }
  if (values["confirmed_inactive_ticket"] != 0) {
    fail("confirmed registrations have inactive tickets: " values["confirmed_inactive_ticket"])
  }
  if (values["duplicate_active_registrations"] != 0) {
    fail("duplicate active registrations found: " values["duplicate_active_registrations"])
  }
  if (values["duplicate_checkins"] != 0) {
    fail("duplicate check-in rows found: " values["duplicate_checkins"])
  }
  if (values["booking_confirmed_audits"] != values["confirmed"]) {
    fail("booking.confirmed audit count does not match confirmed registrations: audits=" values["booking_confirmed_audits"] " confirmed=" values["confirmed"])
  }
  if (values["ticket_issued_audits"] != values["tickets"]) {
    fail("ticket.issued audit count does not match tickets: audits=" values["ticket_issued_audits"] " tickets=" values["tickets"])
  }

  exit status
}
