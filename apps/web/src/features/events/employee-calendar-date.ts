const employeeCalendarTimeZone = "Asia/Taipei";
const employeeCalendarUtcOffsetHours = 8;

const dateKeyFormatter = new Intl.DateTimeFormat("en-US", {
  day: "2-digit",
  month: "2-digit",
  timeZone: employeeCalendarTimeZone,
  year: "numeric",
});

type EmployeeCalendarDateParts = {
  year: number;
  month: number;
  day: number;
};

export function localDateKey(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return [
    String(parts.year).padStart(4, "0"),
    String(parts.month).padStart(2, "0"),
    String(parts.day).padStart(2, "0"),
  ].join("-");
}

export function startOfEmployeeCalendarDay(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return dateFromEmployeeCalendarParts(parts.year, parts.month, parts.day);
}

export function startOfEmployeeCalendarWeek(date: Date) {
  const start = startOfEmployeeCalendarDay(date);
  const day = employeeCalendarWeekdayIndex(start);
  const mondayOffset = day === 0 ? -6 : 1 - day;
  return addEmployeeCalendarDays(start, mondayOffset);
}

export function startOfEmployeeCalendarMonth(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return dateFromEmployeeCalendarParts(parts.year, parts.month, 1);
}

export function addEmployeeCalendarDays(date: Date, days: number) {
  const parts = employeeCalendarDateParts(date);
  return dateFromEmployeeCalendarParts(
    parts.year,
    parts.month,
    parts.day + days,
  );
}

export function parseEmployeeCalendarDateKey(value: string, fallback: Date) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return startOfEmployeeCalendarDay(fallback);
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = dateFromEmployeeCalendarParts(year, month, day);
  return localDateKey(date) === value
    ? date
    : startOfEmployeeCalendarDay(fallback);
}

export function employeeCalendarWeekdayLabel(date: Date) {
  return date.toLocaleDateString("zh-TW", {
    timeZone: employeeCalendarTimeZone,
    weekday: "short",
  });
}

export function employeeCalendarDayNumber(date: Date) {
  return String(employeeCalendarDateParts(date).day);
}

export function employeeCalendarMonthTitle(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return `${parts.year}年${parts.month}月`;
}

export function employeeCalendarMonthKey(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return `${String(parts.year).padStart(4, "0")}-${String(parts.month).padStart(
    2,
    "0",
  )}`;
}

export function formatEmployeeCalendarMonthDay(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return `${String(parts.month).padStart(2, "0")}/${String(parts.day).padStart(
    2,
    "0",
  )}`;
}

export function formatEmployeeCalendarClock(date: Date) {
  return date.toLocaleTimeString("zh-TW", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: employeeCalendarTimeZone,
  });
}

function employeeCalendarDateParts(date: Date): EmployeeCalendarDateParts {
  const entries = new Map(
    dateKeyFormatter.formatToParts(date).map((part) => [part.type, part.value]),
  );
  return {
    day: Number(entries.get("day")),
    month: Number(entries.get("month")),
    year: Number(entries.get("year")),
  };
}

function employeeCalendarWeekdayIndex(date: Date) {
  const parts = employeeCalendarDateParts(date);
  return new Date(Date.UTC(parts.year, parts.month - 1, parts.day)).getUTCDay();
}

function dateFromEmployeeCalendarParts(
  year: number,
  month: number,
  day: number,
) {
  return new Date(
    Date.UTC(year, month - 1, day, -employeeCalendarUtcOffsetHours),
  );
}
