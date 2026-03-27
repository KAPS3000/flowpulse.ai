'use client';

import { useEffect, useRef, useCallback } from 'react';
import * as d3 from 'd3';
import { useStore } from '@/lib/store';
import type { TopologyNode } from '@/lib/api';

interface SimNode extends d3.SimulationNodeDatum {
  id: string;
  data: TopologyNode;
}

interface SimLink extends d3.SimulationLinkDatum<SimNode> {
  bandwidth: number;
}

export function TopologyHeatmap() {
  const svgRef = useRef<SVGSVGElement>(null);
  const nodes = useStore((s) => s.topologyNodes);

  const getNodeColor = useCallback((n: TopologyNode) => {
    const util = Math.max(n.cpu_avg, n.ib_util_pct);
    if (util > 80) return '#ef4444';
    if (util > 60) return '#f59e0b';
    if (util > 30) return '#3b82f6';
    return '#10b981';
  }, []);

  useEffect(() => {
    if (!svgRef.current || nodes.length === 0) return;

    const svg = d3.select(svgRef.current);
    const width = svgRef.current.clientWidth;
    const height = 500;

    svg.selectAll('*').remove();
    svg.attr('viewBox', `0 0 ${width} ${height}`);

    const simNodes: SimNode[] = nodes.map((n) => ({
      id: n.node_id,
      data: n,
    }));

    // Create links between nodes (fully connected for training topology)
    const simLinks: SimLink[] = [];
    for (let i = 0; i < simNodes.length; i++) {
      for (let j = i + 1; j < simNodes.length && j < i + 4; j++) {
        simLinks.push({
          source: simNodes[i],
          target: simNodes[j],
          bandwidth: simNodes[i].data.tx_bytes + simNodes[j].data.tx_bytes,
        });
      }
    }

    const simulation = d3
      .forceSimulation<SimNode>(simNodes)
      .force('link', d3.forceLink<SimNode, SimLink>(simLinks).id((d) => d.id).distance(80))
      .force('charge', d3.forceManyBody().strength(-200))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius(30));

    const g = svg.append('g');

    // Zoom behavior
    svg.call(
      d3.zoom<SVGSVGElement, unknown>()
        .scaleExtent([0.3, 4])
        .on('zoom', (event) => g.attr('transform', event.transform))
    );

    const link = g
      .selectAll<SVGLineElement, SimLink>('line')
      .data(simLinks)
      .join('line')
      .attr('stroke', '#1e293b')
      .attr('stroke-width', 1)
      .attr('stroke-opacity', 0.6);

    const node = g
      .selectAll<SVGGElement, SimNode>('g.node')
      .data(simNodes)
      .join('g')
      .attr('class', 'node')
      .call(
        d3.drag<SVGGElement, SimNode>()
          .on('start', (event, d) => {
            if (!event.active) simulation.alphaTarget(0.3).restart();
            d.fx = d.x;
            d.fy = d.y;
          })
          .on('drag', (event, d) => {
            d.fx = event.x;
            d.fy = event.y;
          })
          .on('end', (event, d) => {
            if (!event.active) simulation.alphaTarget(0);
            d.fx = null;
            d.fy = null;
          })
      );

    node
      .append('circle')
      .attr('r', 18)
      .attr('fill', (d) => getNodeColor(d.data))
      .attr('stroke', '#0a0e1a')
      .attr('stroke-width', 2)
      .attr('opacity', 0.9);

    node
      .append('text')
      .text((d) => d.id.slice(-4))
      .attr('text-anchor', 'middle')
      .attr('dy', 4)
      .attr('fill', 'white')
      .attr('font-size', '9px')
      .attr('font-family', 'monospace');

    // Tooltip
    const tooltip = d3
      .select('body')
      .append('div')
      .attr('class', 'fixed pointer-events-none bg-fp-surface border border-fp-border rounded-lg px-3 py-2 text-xs text-fp-text shadow-lg z-50')
      .style('opacity', 0);

    node
      .on('mouseover', (event, d) => {
        tooltip.transition().duration(150).style('opacity', 1);
        tooltip.html(
          `<strong>${d.data.node_id}</strong><br/>` +
          `CPU: ${d.data.cpu_avg.toFixed(1)}%<br/>` +
          `IB: ${d.data.ib_util_pct.toFixed(1)}%<br/>` +
          `Status: ${d.data.status}`
        );
        tooltip.style('left', event.pageX + 12 + 'px').style('top', event.pageY - 12 + 'px');
      })
      .on('mouseout', () => {
        tooltip.transition().duration(150).style('opacity', 0);
      });

    simulation.on('tick', () => {
      link
        .attr('x1', (d) => (d.source as SimNode).x ?? 0)
        .attr('y1', (d) => (d.source as SimNode).y ?? 0)
        .attr('x2', (d) => (d.target as SimNode).x ?? 0)
        .attr('y2', (d) => (d.target as SimNode).y ?? 0);

      node.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`);
    });

    return () => {
      simulation.stop();
      tooltip.remove();
    };
  }, [nodes, getNodeColor]);

  if (nodes.length === 0) {
    return (
      <div className="card text-center py-16 text-fp-muted">
        No topology data available. Waiting for node metrics...
      </div>
    );
  }

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white">Network Topology Heatmap</h2>
        <div className="flex items-center gap-4 text-xs text-fp-muted">
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-full bg-emerald-500" /> &lt;30%
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-full bg-blue-500" /> 30-60%
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-full bg-amber-500" /> 60-80%
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-full bg-red-500" /> &gt;80%
          </span>
        </div>
      </div>
      <svg ref={svgRef} className="w-full" style={{ height: '500px', background: '#0a0e1a' }} />
    </div>
  );
}
