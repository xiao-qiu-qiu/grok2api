import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Clock3, RefreshCw } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Spinner } from "@/components/ui/spinner";
import { getRequestAuditPerformance, type AuditDTO, type AuditDetailDTO } from "./request-audits-api";
import { formatDateTime, formatDuration } from "@/shared/lib/format";

export function AuditPerformancePopover({ audit, children, onDetails }: { audit: AuditDTO; children: ReactNode; onDetails: () => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const content = useRef<HTMLDivElement>(null);
  const openedByHover = useRef(false);
  const clear = () => { clearTimeout(timer.current); };
  useEffect(() => () => clearTimeout(timer.current), []);
  const changeOpen = (value: boolean) => { clear(); setOpen(value); };
  const enter = () => { clear(); timer.current = setTimeout(() => { if (!open) openedByHover.current = true; setOpen(true); }, 200); };
  const leave = () => {
    clear();
    timer.current = setTimeout(() => {
      if (!content.current?.contains(document.activeElement)) setOpen(false);
    }, 180);
  };
  const detail = useQuery({
    queryKey: ["request-audits", "performance", audit.id],
    queryFn: ({ signal }) => getRequestAuditPerformance(audit.id, signal),
    enabled: open, staleTime: 60_000, gcTime: 60_000, retry: false,
  });

  return <Popover open={open} onOpenChange={changeOpen}>
    <PopoverTrigger asChild>
      <button type="button" aria-description={t("auditTiming.hint")} onMouseEnter={enter} onMouseLeave={leave} onBlur={leave}
        onPointerDown={() => { openedByHover.current = false; }} onKeyDown={(event) => { openedByHover.current = false; if (open && event.key === "ArrowDown") { event.preventDefault(); content.current?.focus(); } }}
        className="-mx-1.5 rounded-md px-1.5 py-1 text-left cursor-help transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        {children}
      </button>
    </PopoverTrigger>
    <PopoverContent ref={content} side="left" align="center" sideOffset={10} collisionPadding={12}
      aria-label={t("auditTiming.title")} onMouseEnter={clear} onMouseLeave={leave} onBlur={leave}
      onOpenAutoFocus={(event) => { if (openedByHover.current) event.preventDefault(); }} onCloseAutoFocus={(event) => { if (openedByHover.current) event.preventDefault(); }}
      className="w-[440px] max-w-[calc(100vw-24px)] max-h-[min(620px,var(--radix-popover-content-available-height))] overflow-y-auto overscroll-contain p-0 text-xs shadow-xl">
      <div className="sticky top-0 z-10 flex items-center justify-between border-b bg-popover px-4 py-3">
        <span className="flex items-center gap-2 font-semibold"><Clock3 className="size-4 text-primary" />{t("auditTiming.title")}</span>
        <span className="font-mono text-[10px] text-muted-foreground">#{audit.id}</span>
      </div>
      <div className="space-y-4 p-4">
        <PerformanceSummary audit={audit} />
        {detail.isPending ? <div className="flex items-center gap-2 py-3 text-muted-foreground"><Spinner className="size-4" />{t("auditTiming.loading")}</div> : null}
        {detail.isError ? <div className="rounded-md bg-destructive/10 p-3 text-destructive"><p>{t("auditTiming.loadError")}</p><Button size="sm" variant="outline" className="mt-2" onClick={() => void detail.refetch()}>{t("auditTiming.retry")}</Button></div> : null}
        {detail.data ? <PerformanceProcess detail={detail.data} /> : null}
      </div>
      <div className="border-t px-4 py-2"><Button variant="ghost" size="sm" className="w-full justify-between text-xs" onClick={() => { changeOpen(false); onDetails(); }}>{t("auditTiming.details")}<ArrowRight className="size-3.5" /></Button></div>
    </PopoverContent>
  </Popover>;
}

function PerformanceSummary({ audit }: { audit: AuditDTO }) {
  const { t } = useTranslation();
  const first = audit.streaming && audit.firstTokenMs !== undefined ? audit.firstTokenMs : undefined;
  const firstLabel = !audit.streaming ? t("audits.firstTokenNotApplicable") : first === undefined ? t("audits.performanceNotRecorded") : formatDuration(first);
  return <div>
    <div className="grid grid-cols-3 gap-2">
      {[ [t("auditTiming.total"), formatDuration(audit.durationMs)], [t("auditTiming.first"), firstLabel], [t("auditTiming.after"), first === undefined ? "—" : formatDuration(Math.max(0, audit.durationMs - first))] ].map(([label, value]) =>
        <div key={label} className="rounded-lg bg-muted/60 p-2.5"><div className="text-[10px] text-muted-foreground">{label}</div><div className="mt-1 font-semibold tabular-nums">{value}</div></div>)}
    </div>
    {first !== undefined && audit.durationMs > 0 ? <div className="mt-2 flex h-1.5 overflow-hidden rounded-full bg-emerald-500/30" aria-hidden="true"><span className="bg-amber-500/75" style={{ width: `${Math.min(100, first / audit.durationMs * 100)}%` }} /></div> : null}
    <p className="mt-2 text-[10px] leading-relaxed text-muted-foreground">{!audit.streaming ? `${t("audits.nonStreamFirstTokenHint")} ` : ""}{t(audit.streaming ? "audits.streamThroughputHint" : "audits.averageThroughputHint")}</p>
  </div>;
}

export function PerformanceProcess({ detail }: { detail: AuditDetailDTO }) {
  const { t, i18n } = useTranslation();
  const { audit, performance, attempts } = detail;
  const calls = performance?.calls ?? [];
  const switches = calls.reduce((count, call, index) => count + (index > 0 && call.accountId && calls[index - 1].accountId && call.accountId !== calls[index - 1].accountId ? 1 : 0), 0);
  const ms = (value: number | undefined) => value === undefined ? "—" : formatDuration(value);
  const label = (key: string) => t(`auditTiming.${key}`, { defaultValue: key });
  return <>
    {performance ? <>
      <div className="grid grid-cols-2 gap-x-5 gap-y-2">
        {([ ["selection", performance.selectionMs], ["credential", performance.credentialMs], ["upstream", performance.upstreamMs], ["quality", performance.qualityMs] ] as const).map(([key, value]) =>
          <div key={key} className="flex justify-between gap-2"><span className="text-muted-foreground">{label(key)}</span><span className="font-medium tabular-nums">{ms(value)}</span></div>)}
      </div>
      <p className="text-[10px] leading-relaxed text-muted-foreground">{t("auditTiming.scope")}</p>
      <div className="flex flex-wrap gap-1.5 text-[10px]">
        <span className="rounded bg-muted px-2 py-1">{t("auditTiming.calls", { count: calls.length })}</span>
        <span className="rounded bg-muted px-2 py-1">{t("auditTiming.retries", { count: Math.max(0, calls.length - 1) })}</span>
        <span className="rounded bg-muted px-2 py-1">{t("auditTiming.switched", { count: switches })}</span>
        <span className="rounded bg-muted px-2 py-1">{t("auditTiming.diagnostics", { count: attempts.length })}</span>
      </div>
      <div className="space-y-3">
        <h4 className="font-semibold">{t("auditTiming.timeline")}</h4>
        {calls.length === 0 ? <p className="text-muted-foreground">{t("auditTiming.noCalls")}</p> : null}
        {calls.map((call, index) => <div key={call.number} className="relative border-l-2 border-muted pl-3">
          <span className="absolute -left-[5px] top-1.5 size-2 rounded-full bg-primary/70" />
          <div className="flex items-start justify-between gap-2"><span className="min-w-0 break-all font-medium">{index > 0 && call.accountId && calls[index - 1].accountId && call.accountId !== calls[index - 1].accountId ? <RefreshCw className="mr-1 inline size-3 text-amber-500" /> : null}#{call.number} · {call.accountName || `#${call.accountId ?? "—"}`}</span><span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">{t("auditTiming.begin", { time: ms(call.startedOffsetMs) })}</span></div>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground"><span>{label(call.outcome)}{call.action ? ` → ${label(`action_${call.action}`)}` : ""}{call.statusCode ? ` · HTTP ${call.statusCode}` : ""}</span><span>{t("auditTiming.upstream")} {ms(call.upstreamMs)}</span>{call.qualityMs !== undefined ? <span>{t("auditTiming.quality")} {ms(call.qualityMs)}</span> : null}</div>
          {call.qualityMs !== undefined ? <div className="mt-2 grid grid-cols-3 gap-1 rounded bg-muted/50 p-2 text-[10px]">{([ ["byte", call.firstByteMs], ["thinking", call.firstThinkingMs], ["visible", call.firstVisibleMs] ] as const).map(([key, value]) => <div key={key}><div className="text-muted-foreground">{label(key)}</div><div className="mt-0.5 tabular-nums">{ms(value)}</div></div>)}</div> : null}
        </div>)}
        <p className="text-[10px] leading-relaxed text-muted-foreground">{t("auditTiming.signalScope")} {t("auditTiming.callScope")}</p>
      </div>
    </> : <p className="rounded-lg bg-amber-500/10 p-3 leading-relaxed text-amber-700 dark:text-amber-300">{t("auditTiming.legacy")}</p>}
    {attempts.length > 0 ? <div className="space-y-2">
      <h4 className="font-semibold">{t("auditTiming.failure")}</h4>
      {attempts.map((attempt) => <div key={attempt.id || attempt.number} className="rounded-lg border bg-muted/20 p-2.5">
        <div className="flex justify-between gap-2"><span className="min-w-0 break-all font-medium">#{attempt.number} · {attempt.accountName || `#${attempt.accountId ?? "—"}`}</span><span className="shrink-0 tabular-nums">{ms(attempt.durationMs)}</span></div>
        <div className="mt-1 text-[10px] text-muted-foreground">{formatDateTime(attempt.startedAt, i18n.language)} · {label(attempt.stage)}{attempt.upstreamStatusCode ? ` · HTTP ${attempt.upstreamStatusCode}` : ""}</div>
        <p className="mt-1 break-words text-[11px] leading-relaxed text-amber-700 dark:text-amber-300">{(attempt.transportError || attempt.errorChain[0]?.message || attempt.upstreamStatus || label(attempt.stage)).slice(0, 300)}</p>
      </div>)}
    </div> : !performance ? <p className="text-muted-foreground">{t("auditTiming.none")}</p> : null}
    <div className="border-t pt-3"><div className="flex justify-between gap-2"><span className="text-muted-foreground">{t("auditTiming.result")}</span><span className={audit.errorCode || audit.statusCode >= 400 ? "text-destructive" : "text-emerald-600 dark:text-emerald-400"}>{audit.errorCode || `HTTP ${audit.statusCode}`}</span></div><div className="mt-1 break-all text-muted-foreground">{t("auditTiming.finalAccount")} · {audit.accountName || (audit.accountId ? `#${audit.accountId}` : "—")}</div></div>
  </>;
}
