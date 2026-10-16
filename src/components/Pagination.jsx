import React from "react";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { fmtNum } from "@/lib/utils";
import { PAGE_SIZES } from "@/lib/state";

export function Pagination({ page, size, total, shown, onPage, onSize }) {
  const pages = total == null ? null : Math.max(1, Math.ceil(total / size));
  const from = shown ? (page - 1) * size + 1 : 0;
  const to = (page - 1) * size + shown;
  const hasNext = pages == null ? shown === size : page < pages;

  return (
    <div className="flex items-center justify-between gap-4 border-t border-line px-5 py-2 text-xs text-muted">
      <div className="tabular-nums">
        {fmtNum(from)}–{fmtNum(to)} <span className="text-faint">of</span>{" "}
        <span className="font-medium text-fg">{total == null ? "?" : fmtNum(total)}</span>
      </div>
      <div className="flex items-center gap-1">
        <Select
          wrapperClassName="mr-2"
          className="h-8 w-auto text-xs"
          value={size}
          onChange={(e) => onSize(Number(e.target.value))}
          title="Rows per page"
        >
          {PAGE_SIZES.map((s) => (
            <option key={s} value={s}>
              {s} / page
            </option>
          ))}
        </Select>
        <Button size="icon" variant="ghost" disabled={page <= 1} onClick={() => onPage(1)} title="First">
          <ChevronsLeft size={15} />
        </Button>
        <Button size="icon" variant="ghost" disabled={page <= 1} onClick={() => onPage(page - 1)} title="Previous">
          <ChevronLeft size={15} />
        </Button>
        <span className="px-2 tabular-nums">
          <span className="font-medium text-fg">{page}</span>
          {pages != null && <span className="text-faint"> / {fmtNum(pages)}</span>}
        </span>
        <Button size="icon" variant="ghost" disabled={!hasNext} onClick={() => onPage(page + 1)} title="Next">
          <ChevronRight size={15} />
        </Button>
        <Button size="icon" variant="ghost" disabled={pages == null || page >= pages} onClick={() => onPage(pages)} title="Last">
          <ChevronsRight size={15} />
        </Button>
      </div>
    </div>
  );
}
