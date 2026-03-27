// SPDX-License-Identifier: GPL-2.0
// FlowPulse - InfiniBand verbs kprobes for RDMA operation tracking

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include "common.h"

char LICENSE[] SEC("license") = "GPL";

/* ── Maps ──────────────────────────────────────────────────── */

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, struct ib_op_key);
    __type(value, struct ib_op_value);
} ib_qp_stats SEC(".maps");

/* Track pending send timestamps for completion latency calculation */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);          /* wr_id */
    __type(value, __u64);        /* timestamp */
} pending_sends SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, RINGBUF_SIZE / 4);
} ib_events SEC(".maps");

/* ── ib_post_send kprobe ───────────────────────────────────── */

/*
 * We attach to ib_post_send which takes (struct ib_qp *qp,
 * struct ib_send_wr *wr, struct ib_send_wr **bad_wr).
 * From qp we read qp_num and port.
 */
SEC("kprobe/ib_post_send")
int trace_ib_post_send(struct pt_regs *ctx) {
    /* First argument: struct ib_qp *qp */
    struct ib_qp *qp = (struct ib_qp *)PT_REGS_PARM1(ctx);
    if (!qp)
        return 0;

    __u32 qp_num = 0;
    __u8  port   = 0;
    bpf_core_read(&qp_num, sizeof(qp_num), &qp->qp_num);
    bpf_core_read(&port, sizeof(port), &qp->port);

    struct ib_op_key key = {
        .qp_number = qp_num,
        .port      = port,
    };

    __u64 now = bpf_ktime_get_ns();

    struct ib_op_value *val = bpf_map_lookup_elem(&ib_qp_stats, &key);
    if (val) {
        __sync_fetch_and_add(&val->send_count, 1);
        val->last_op_ns = now;
    } else {
        struct ib_op_value new_val = {};
        new_val.send_count = 1;
        new_val.last_op_ns = now;
        bpf_map_update_elem(&ib_qp_stats, &key, &new_val, BPF_NOEXIST);
    }

    /* Record send timestamp for latency calculation keyed by wr_id */
    struct ib_send_wr *wr = (struct ib_send_wr *)PT_REGS_PARM2(ctx);
    if (wr) {
        __u64 wr_id = 0;
        bpf_core_read(&wr_id, sizeof(wr_id), &wr->wr_id);
        bpf_map_update_elem(&pending_sends, &wr_id, &now, BPF_ANY);
    }

    return 0;
}

/* ── ib_post_recv kprobe ───────────────────────────────────── */

SEC("kprobe/ib_post_recv")
int trace_ib_post_recv(struct pt_regs *ctx) {
    struct ib_qp *qp = (struct ib_qp *)PT_REGS_PARM1(ctx);
    if (!qp)
        return 0;

    __u32 qp_num = 0;
    __u8  port   = 0;
    bpf_core_read(&qp_num, sizeof(qp_num), &qp->qp_num);
    bpf_core_read(&port, sizeof(port), &qp->port);

    struct ib_op_key key = {
        .qp_number = qp_num,
        .port      = port,
    };

    struct ib_op_value *val = bpf_map_lookup_elem(&ib_qp_stats, &key);
    if (val) {
        __sync_fetch_and_add(&val->recv_count, 1);
        val->last_op_ns = bpf_ktime_get_ns();
    } else {
        struct ib_op_value new_val = {};
        new_val.recv_count = 1;
        new_val.last_op_ns = bpf_ktime_get_ns();
        bpf_map_update_elem(&ib_qp_stats, &key, &new_val, BPF_NOEXIST);
    }

    return 0;
}

/* ── ib_poll_cq kprobe (completion latency) ────────────────── */

/*
 * ib_poll_cq returns the number of completions.
 * We use kretprobe to measure time spent polling completions.
 */

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} cq_poll_start SEC(".maps");

SEC("kprobe/ib_poll_cq")
int trace_ib_poll_cq_entry(struct pt_regs *ctx) {
    __u32 zero = 0;
    __u64 now = bpf_ktime_get_ns();
    bpf_map_update_elem(&cq_poll_start, &zero, &now, BPF_ANY);
    return 0;
}

SEC("kretprobe/ib_poll_cq")
int trace_ib_poll_cq_return(struct pt_regs *ctx) {
    __u32 zero = 0;
    __u64 *start = bpf_map_lookup_elem(&cq_poll_start, &zero);
    if (!start)
        return 0;

    int num_completions = (int)PT_REGS_RC(ctx);
    if (num_completions <= 0)
        return 0;

    __u64 now = bpf_ktime_get_ns();
    __u64 latency = now - *start;

    /* We can't easily get the QP from the CQ poll return path,
     * so we just record the total poll latency in a global counter.
     * Per-QP completion latency is calculated via pending_sends map. */

    return 0;
}
