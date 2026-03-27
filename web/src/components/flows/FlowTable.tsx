'use client';

import { useMemo, useRef } from 'react';
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useState } from 'react';
import { useStore } from '@/lib/store';
import { ipToString, formatBytes, protocolName, type Flow } from '@/lib/api';

export function FlowTable() {
  const flows = useStore((s) => s.flows);
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState('');
  const parentRef = useRef<HTMLDivElement>(null);

  const columns = useMemo<ColumnDef<Flow>[]>(
    () => [
      {
        accessorKey: 'key.src_ip',
        header: 'Source',
        cell: ({ row }) =>
          `${ipToString(row.original.key.src_ip)}:${row.original.key.src_port}`,
        size: 180,
      },
      {
        accessorKey: 'key.dst_ip',
        header: 'Destination',
        cell: ({ row }) =>
          `${ipToString(row.original.key.dst_ip)}:${row.original.key.dst_port}`,
        size: 180,
      },
      {
        accessorKey: 'key.protocol',
        header: 'Proto',
        cell: ({ row }) => protocolName(row.original.key.protocol),
        size: 70,
      },
      {
        accessorKey: 'bytes',
        header: 'Bytes',
        cell: ({ row }) => formatBytes(row.original.bytes),
        size: 100,
      },
      {
        accessorKey: 'packets',
        header: 'Packets',
        cell: ({ row }) => row.original.packets.toLocaleString(),
        size: 100,
      },
      {
        accessorKey: 'node_id',
        header: 'Node',
        size: 120,
      },
      {
        id: 'rdma',
        header: 'RDMA QP',
        cell: ({ row }) => (row.original.rdma ? `QP ${row.original.rdma.qp_number}` : '-'),
        size: 80,
      },
      {
        id: 'ecn',
        header: 'ECN/CNP',
        cell: ({ row }) =>
          row.original.rdma
            ? `${row.original.rdma.ecn_marks}/${row.original.rdma.cnp_count}`
            : '-',
        size: 90,
      },
    ],
    []
  );

  const table = useReactTable({
    data: flows,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  });

  const { rows } = table.getRowModel();

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 36,
    overscan: 20,
  });

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white">
          Flows ({flows.length.toLocaleString()})
        </h2>
        <input
          type="text"
          placeholder="Filter flows..."
          value={globalFilter}
          onChange={(e) => setGlobalFilter(e.target.value)}
          className="bg-fp-bg border border-fp-border rounded-lg px-3 py-1.5 text-sm text-fp-text placeholder-fp-muted focus:outline-none focus:ring-1 focus:ring-fp-accent"
        />
      </div>

      <div ref={parentRef} className="overflow-auto" style={{ height: '600px' }}>
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-fp-surface z-10">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id}>
                {hg.headers.map((header) => (
                  <th
                    key={header.id}
                    onClick={header.column.getToggleSortingHandler()}
                    className="text-left px-3 py-2 text-fp-muted font-medium cursor-pointer select-none border-b border-fp-border hover:text-white"
                    style={{ width: header.getSize() }}
                  >
                    {flexRender(header.column.columnDef.header, header.getContext())}
                    {{ asc: ' \u2191', desc: ' \u2193' }[
                      header.column.getIsSorted() as string
                    ] ?? ''}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {virtualizer.getVirtualItems().length === 0 && (
              <tr>
                <td colSpan={columns.length} className="text-center py-12 text-fp-muted">
                  No flows yet. Waiting for data...
                </td>
              </tr>
            )}
            <tr style={{ height: `${virtualizer.getVirtualItems()[0]?.start ?? 0}px` }} />
            {virtualizer.getVirtualItems().map((vRow) => {
              const row = rows[vRow.index];
              return (
                <tr
                  key={row.id}
                  className="hover:bg-fp-bg/50 transition-colors border-b border-fp-border/50"
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-3 py-1.5 whitespace-nowrap">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              );
            })}
            <tr
              style={{
                height: `${
                  virtualizer.getTotalSize() -
                  (virtualizer.getVirtualItems().at(-1)?.end ?? 0)
                }px`,
              }}
            />
          </tbody>
        </table>
      </div>
    </div>
  );
}
