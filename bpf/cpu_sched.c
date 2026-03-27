// SPDX-License-Identifier: GPL-2.0
// FlowPulse - CPU scheduling and softirq tracepoints

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include "common.h"

char LICENSE[] SEC("license") = "GPL";

/* ── Maps ──────────────────────────────────────────────────── */

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, MAX_CPUS);
    __type(key, __u32);
    __type(value, struct cpu_value);
} cpu_stats SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, MAX_CPUS);
    __type(key, __u32);
    __type(value, struct softirq_value);
} softirq_stats SEC(".maps");

/* Per-CPU scratch space for softirq entry timestamps */
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} softirq_start SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, RINGBUF_SIZE / 4);
} cpu_events SEC(".maps");

/* ── sched_switch tracepoint ───────────────────────────────── */

/* /sys/kernel/debug/tracing/events/sched/sched_switch/format */
struct sched_switch_args {
    /* common tracepoint fields */
    unsigned short common_type;
    unsigned char  common_flags;
    unsigned char  common_preempt_count;
    int            common_pid;

    char           prev_comm[16];
    pid_t          prev_pid;
    int            prev_prio;
    long           prev_state;
    char           next_comm[16];
    pid_t          next_pid;
    int            next_prio;
};

SEC("tracepoint/sched/sched_switch")
int handle_sched_switch(struct sched_switch_args *ctx) {
    __u32 cpu = bpf_get_smp_processor_id();
    __u64 now = bpf_ktime_get_ns();

    struct cpu_value *val = bpf_map_lookup_elem(&cpu_stats, &cpu);
    if (val) {
        __u64 delta = now - val->last_switch_ns;

        /* prev task was ON cpu, now going OFF */
        if (ctx->prev_pid != 0)
            __sync_fetch_and_add(&val->on_cpu_ns, delta);
        else
            __sync_fetch_and_add(&val->off_cpu_ns, delta);

        /* Track voluntary vs involuntary context switches */
        if (ctx->prev_state == 0)
            __sync_fetch_and_add(&val->involuntary_switches, 1);
        else
            __sync_fetch_and_add(&val->voluntary_switches, 1);

        val->last_switch_ns = now;
    } else {
        struct cpu_value new_val = {};
        new_val.last_switch_ns = now;
        bpf_map_update_elem(&cpu_stats, &cpu, &new_val, BPF_NOEXIST);
    }

    return 0;
}

/* ── softirq tracepoints ──────────────────────────────────── */

#define NET_TX_SOFTIRQ 2
#define NET_RX_SOFTIRQ 3

struct softirq_args {
    unsigned short common_type;
    unsigned char  common_flags;
    unsigned char  common_preempt_count;
    int            common_pid;
    unsigned int   vec;
};

SEC("tracepoint/irq/softirq_entry")
int handle_softirq_entry(struct softirq_args *ctx) {
    __u32 zero = 0;
    __u64 now = bpf_ktime_get_ns();
    bpf_map_update_elem(&softirq_start, &zero, &now, BPF_ANY);
    return 0;
}

SEC("tracepoint/irq/softirq_exit")
int handle_softirq_exit(struct softirq_args *ctx) {
    __u32 zero = 0;
    __u64 *start = bpf_map_lookup_elem(&softirq_start, &zero);
    if (!start)
        return 0;

    __u64 now = bpf_ktime_get_ns();
    __u64 delta = now - *start;

    __u32 cpu = bpf_get_smp_processor_id();
    struct softirq_value *val = bpf_map_lookup_elem(&softirq_stats, &cpu);
    if (val) {
        __sync_fetch_and_add(&val->total_ns, delta);
        __sync_fetch_and_add(&val->count, 1);

        if (ctx->vec == NET_RX_SOFTIRQ)
            __sync_fetch_and_add(&val->net_rx_ns, delta);
        else if (ctx->vec == NET_TX_SOFTIRQ)
            __sync_fetch_and_add(&val->net_tx_ns, delta);
    } else {
        struct softirq_value new_val = {};
        new_val.total_ns = delta;
        new_val.count    = 1;
        if (ctx->vec == NET_RX_SOFTIRQ)
            new_val.net_rx_ns = delta;
        else if (ctx->vec == NET_TX_SOFTIRQ)
            new_val.net_tx_ns = delta;
        bpf_map_update_elem(&softirq_stats, &cpu, &new_val, BPF_NOEXIST);
    }

    return 0;
}
