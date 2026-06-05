import { useState } from "react";
import { Field, SegmentedFilter, SelectField } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { Option } from "@/lib/ui/options";
import {
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
  const filtersActive =
    discovery.capacity !== "all" ||
    discovery.city !== "all" ||
    discovery.status !== "all";
  const [filtersOpen, setFiltersOpen] = useState(false);
  const filtersExpanded = filtersOpen || filtersActive;
  return (
    <section
      className={[
        "employee-discovery-strip",
        active ? "has-active-discovery" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      aria-label="活動搜尋與篩選"
    >
      <div className="employee-discovery-head">
        <p className="employee-discovery-summary" aria-live="polite">
          {summary}
        </p>
      </div>
      <div className="employee-discovery-primary">
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
        <Button
          aria-controls="employee-discovery-filters"
          aria-expanded={filtersExpanded}
          className="employee-discovery-filter-toggle"
          size="sm"
          type="button"
          variant={filtersActive ? "secondary" : "outline"}
          onClick={() => setFiltersOpen((open) => !open)}
        >
          <Icon name="sliders" />
          篩選
        </Button>
      </div>
      <div
        className={[
          "employee-discovery-controls",
          filtersExpanded ? "is-open" : "",
        ]
          .filter(Boolean)
          .join(" ")}
        id="employee-discovery-filters"
      >
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
      </div>
    </section>
  );
}
