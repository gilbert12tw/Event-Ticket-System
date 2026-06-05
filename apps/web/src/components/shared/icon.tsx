import type { LucideIcon } from "lucide-react";
import {
  Activity,
  ArrowRight,
  Ban,
  BarChart3,
  Bell,
  CalendarDays,
  CalendarPlus,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  Copy,
  Database,
  Download,
  Eye,
  EyeOff,
  FileSearch,
  LogOut,
  MapPin,
  Play,
  Plus,
  RefreshCw,
  Save,
  ScanLine,
  Send,
  Settings,
  SlidersHorizontal,
  Ticket as TicketIcon,
  Trash2,
  Users,
  WifiOff,
  X,
} from "lucide-react";
import type { IconName } from "@/app/routes";

const iconComponents: Record<IconName, LucideIcon> = {
  activity: Activity,
  arrowRight: ArrowRight,
  audit: FileSearch,
  ban: Ban,
  bell: Bell,
  calendar: CalendarDays,
  calendarPlus: CalendarPlus,
  chart: BarChart3,
  chevronLeft: ChevronLeft,
  chevronRight: ChevronRight,
  clipboard: ClipboardList,
  copy: Copy,
  database: Database,
  download: Download,
  eye: Eye,
  eyeOff: EyeOff,
  logout: LogOut,
  mapPin: MapPin,
  play: Play,
  plus: Plus,
  refresh: RefreshCw,
  save: Save,
  scan: ScanLine,
  send: Send,
  settings: Settings,
  sliders: SlidersHorizontal,
  ticket: TicketIcon,
  trash: Trash2,
  users: Users,
  wifiOff: WifiOff,
  x: X,
};

export function Icon({ name }: Readonly<{ name: IconName }>) {
  const Component = iconComponents[name];
  return (
    <Component
      aria-hidden="true"
      className="icon"
      size={16}
      strokeWidth={2}
      absoluteStrokeWidth
    />
  );
}
