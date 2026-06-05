import {
  Field,
  SegmentedFilter,
  SelectField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { Option } from "@/lib/ui/options";
import {
  defaultEmployeeDiscoveryState,
  discoveryIsActive,
  employeeDiscoveryCapacityOptions,
  employeeDiscoveryStatusOptions,
  type EmployeeDiscoveryState,
} from "./employee-discovery";

type EmployeeDiscoveryControlsProps = {
  cityOptions: Option[];
  discovery: EmployeeDiscoveryState;
  summary: string;
  onChange: (next: EmployeeDiscoveryState) => void;
};

export function EmployeeDiscoveryControls({
  cityOptions,
  discovery,
  summary,
  onChange,
}: Readonly<EmployeeDiscoveryControlsProps>) {
  const active = discoveryIsActive(discovery);
  return (
    <section className="employee-discovery-strip" aria-label="活動搜尋與篩選">
      <Field
        autoComplete="off"
        className="employee-discovery-search"
        label="搜尋活動"
        name="event-search"
        placeholder="搜尋活動、地點或標籤…"
        type="search"
        value={discovery.q}
        onChange={(q) => onChange({ ...discovery, q })}
      />
      <div className="employee-discovery-controls">
        <SegmentedFilter
          label="活動狀態"
          options={employeeDiscoveryStatusOptions}
          value={discovery.status}
          onChange={(status) =>
            onChange({
              ...discovery,
              status: status as EmployeeDiscoveryState["status"],
            })
          }
        />
        <SelectField
          className="employee-discovery-select"
          label="地點"
          name="event-city"
          options={cityOptions}
          value={discovery.city}
          onChange={(city) => onChange({ ...discovery, city })}
        />
        <SelectField
          className="employee-discovery-select"
          label="名額"
          name="event-capacity"
          options={employeeDiscoveryCapacityOptions}
          value={discovery.capacity}
          onChange={(capacity) =>
            onChange({
              ...discovery,
              capacity: capacity as EmployeeDiscoveryState["capacity"],
            })
          }
        />
        {active && (
          <Button
            className="employee-discovery-clear"
            size="sm"
            type="button"
            variant="ghost"
            onClick={() => onChange(defaultEmployeeDiscoveryState)}
          >
            <Icon name="x" />
            清除
          </Button>
        )}
      </div>
      <p className="employee-discovery-summary" aria-live="polite">
        {summary}
      </p>
    </section>
  );
}
