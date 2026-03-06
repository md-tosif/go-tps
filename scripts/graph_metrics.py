#!/usr/bin/env python3
"""
Transaction Metrics Graph Generator
Generates graphs for TPS (Transactions Per Second) and Latency metrics
over 1-second intervals from the transactions database.
"""

import sqlite3
import sys
import os
import re
from datetime import datetime, timedelta
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
from collections import defaultdict
import statistics


def parse_timestamp(ts_str):
    """Parse timestamp string that may have nanosecond precision or timezone info.
    Python datetime only supports up to microseconds (6 decimal places).
    Truncates excess fractional digits before parsing.
    """
    if not ts_str:
        return None
    # Truncate fractional seconds beyond 6 digits (nanoseconds -> microseconds)
    ts_str = re.sub(r'(\.\d{6})\d+', r'\1', ts_str)
    return datetime.fromisoformat(ts_str)

DB_PATH = "./load-2.db"
INTERVAL_SECONDS = 5
OUTPUT_DIR = "images"


def ensure_output_dir():
    """Create the output directory if it doesn't exist."""
    if not os.path.exists(OUTPUT_DIR):
        os.makedirs(OUTPUT_DIR)
        print(f"Created output directory: {OUTPUT_DIR}/")


def get_batch_list(conn):
    """Get list of all batches in the database."""
    cursor = conn.cursor()
    cursor.execute("SELECT DISTINCT batch_number FROM transactions ORDER BY batch_number DESC")
    batches = [row[0] for row in cursor.fetchall()]
    return batches


def calculate_tps_intervals(conn, batch_number=None):
    """
    Calculate TPS for both submission and confirmation over 1-second intervals.
    
    Returns:
        submission_data: dict of {timestamp: tps}
        confirmation_data: dict of {timestamp: tps}
    """
    cursor = conn.cursor()
    
    # Query to get submission and confirmation times
    if batch_number:
        query = """
            SELECT submitted_at, confirmed_at, status
            FROM transactions
            WHERE batch_number = ?
            ORDER BY submitted_at
        """
        cursor.execute(query, (batch_number,))
    else:
        query = """
            SELECT submitted_at, confirmed_at, status
            FROM transactions
            ORDER BY submitted_at
        """
        cursor.execute(query)
    
    rows = cursor.fetchall()
    
    if not rows:
        print("No transactions found.")
        return {}, {}
    
    # Parse timestamps and group into intervals
    submission_intervals = defaultdict(int)
    confirmation_intervals = defaultdict(int)
    
    for row in rows:
        submitted_str, confirmed_str, status = row
        
        # Parse submission time
        try:
            submitted_dt = parse_timestamp(submitted_str)
            # Round down to nearest 1-second interval
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            submission_intervals[interval_start] += 1
        except (ValueError, TypeError):
            continue
        
        # Parse confirmation time (if available and successful)
        if confirmed_str and status == 'success':
            try:
                confirmed_dt = parse_timestamp(confirmed_str)
                # Round down to nearest interval (INTERVAL_SECONDS)
                interval_start = confirmed_dt.replace(microsecond=0)
                interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
                confirmation_intervals[interval_start] += 1
            except (ValueError, TypeError):
                continue
    
    # Convert counts to TPS (transactions per second over the interval)
    submission_tps = {ts: count / INTERVAL_SECONDS for ts, count in submission_intervals.items()}
    confirmation_tps = {ts: count / INTERVAL_SECONDS for ts, count in confirmation_intervals.items()}
    
    return submission_tps, confirmation_tps


def calculate_latency_intervals(conn, batch_number=None):
    """
    Calculate average latency for both execution and confirmation over 1-second intervals.
    
    Execution latency: Time for the eth_sendRawTransaction RPC call to return (execution_time in DB, ~ms range)
    Confirmation latency: Time from submission to block inclusion (confirmed_at - submitted_at, ~seconds range)
    
    Returns:
        execution_latency: dict of {timestamp: avg_latency_ms}
        confirmation_latency: dict of {timestamp: avg_latency_ms}
    """
    cursor = conn.cursor()
    
    # Query to get submission time, confirmation time, and execution time
    if batch_number:
        query = """
            SELECT submitted_at, confirmed_at, execution_time, status
            FROM transactions
            WHERE batch_number = ?
            ORDER BY submitted_at
        """
        cursor.execute(query, (batch_number,))
    else:
        query = """
            SELECT submitted_at, confirmed_at, execution_time, status
            FROM transactions
            ORDER BY submitted_at
        """
        cursor.execute(query)
    
    rows = cursor.fetchall()
    
    if not rows:
        print("No transactions found.")
        return {}, {}
    
    # Group latencies by time intervals
    execution_intervals = defaultdict(list)
    confirmation_intervals = defaultdict(list)
    
    for row in rows:
        submitted_str, confirmed_str, exec_time, status = row
        
        try:
            submitted_dt = parse_timestamp(submitted_str)
            # Round down to nearest interval (INTERVAL_SECONDS)
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            
            # Execution latency (from execution_time in DB - time to execute submission)
            if exec_time is not None and exec_time > 0:
                execution_intervals[interval_start].append(exec_time)
            
            # Confirmation latency (time from submission to confirmation)
            if confirmed_str and status == 'success':
                try:
                    confirmed_dt = parse_timestamp(confirmed_str)
                    confirmation_latency_ms = (confirmed_dt - submitted_dt).total_seconds() * 1000
                    if confirmation_latency_ms > 0:
                        confirmation_intervals[interval_start].append(confirmation_latency_ms)
                except (ValueError, TypeError):
                    continue
        except (ValueError, TypeError):
            continue
    
    # Calculate average latency for each interval
    execution_avg = {ts: statistics.mean(latencies) if latencies else 0 
                     for ts, latencies in execution_intervals.items()}
    confirmation_avg = {ts: statistics.mean(latencies) if latencies else 0 
                        for ts, latencies in confirmation_intervals.items()}
    
    return execution_avg, confirmation_avg


def plot_tps_graph(submission_tps, confirmation_tps, batch_number=None):
    """Create and save the TPS graph."""
    if not submission_tps and not confirmation_tps:
        print("No data to plot.")
        return
    
    # Prepare data for plotting
    all_times = sorted(set(list(submission_tps.keys()) + list(confirmation_tps.keys())))
    
    submission_values = [submission_tps.get(t, 0) for t in all_times]
    confirmation_values = [confirmation_tps.get(t, 0) for t in all_times]
    
    # Create the plot
    fig, ax = plt.subplots(figsize=(14, 7))
    
    # Plot both lines
    ax.plot(all_times, submission_values, label='Submission TPS', 
            color='#2196F3', linewidth=2, marker='o', markersize=4)
    ax.plot(all_times, confirmation_values, label='Confirmation TPS',
            color='#4CAF50', linewidth=2, marker='s', markersize=4)
    
    # Formatting
    ax.set_xlabel('Time', fontsize=12, fontweight='bold')
    ax.set_ylabel('Transactions Per Second (TPS)', fontsize=12, fontweight='bold')
    
    title = f'TPS Over Time ({INTERVAL_SECONDS}s intervals)'
    if batch_number:
        title += f'\nBatch: {batch_number}'
    ax.set_title(title, fontsize=14, fontweight='bold', pad=20)
    
    # Format x-axis
    ax.xaxis.set_major_formatter(mdates.DateFormatter('%H:%M:%S'))
    ax.xaxis.set_major_locator(mdates.AutoDateLocator())
    plt.xticks(rotation=45, ha='right')
    
    # Grid
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)
    
    # Legend
    ax.legend(loc='best', fontsize=11, framealpha=0.9)
    
    # Add statistics text box
    if submission_values:
        # Use only non-zero intervals to avoid averaging in idle periods
        sub_nonzero = [v for v in submission_values if v > 0]
        sub_avg = sum(sub_nonzero) / len(sub_nonzero) if sub_nonzero else 0
        sub_max = max(sub_nonzero) if sub_nonzero else 0
        conf_nonzero = [v for v in confirmation_values if v > 0]
        conf_avg = sum(conf_nonzero) / len(conf_nonzero) if conf_nonzero else 0
        conf_max = max(conf_nonzero) if conf_nonzero else 0
        
        stats_text = f'Submission:  Avg: {sub_avg:.2f} TPS  |  Max: {sub_max:.2f} TPS\n'
        stats_text += f'Confirmation: Avg: {conf_avg:.2f} TPS  |  Max: {conf_max:.2f} TPS'
        
        ax.text(0.02, 0.98, stats_text,
                transform=ax.transAxes,
                fontsize=10,
                verticalalignment='top',
                bbox=dict(boxstyle='round', facecolor='wheat', alpha=0.8))
    
    plt.tight_layout()
    
    # Save the graph
    ensure_output_dir()
    output_file = os.path.join(OUTPUT_DIR, f'tps_graph_{batch_number if batch_number else "all"}.png')
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"✓ TPS graph saved to: {output_file}")
    
    # Close the plot to free memory
    plt.close()


def plot_latency_graph(execution_latency, confirmation_latency, batch_number=None):
    """Create and save the latency graph."""
    if not execution_latency and not confirmation_latency:
        print("No data to plot.")
        return
    
    # Prepare data for plotting
    all_times = sorted(set(list(execution_latency.keys()) + list(confirmation_latency.keys())))
    
    execution_values = [execution_latency.get(t, 0) for t in all_times]
    confirmation_values = [confirmation_latency.get(t, 0) for t in all_times]
    
    # Create the plot
    fig, ax = plt.subplots(figsize=(14, 7))
    
    # Plot both lines
    ax.plot(all_times, execution_values, label='Execution Latency', 
            color='#FF9800', linewidth=2, marker='o', markersize=4)
    ax.plot(all_times, confirmation_values, label='Confirmation Latency',
            color='#9C27B0', linewidth=2, marker='s', markersize=4)
    
    # Formatting
    ax.set_xlabel('Time', fontsize=12, fontweight='bold')
    ax.set_ylabel('Latency (milliseconds)', fontsize=12, fontweight='bold')
    
    title = f'Transaction Latency Over Time ({INTERVAL_SECONDS}s intervals)'
    if batch_number:
        title += f'\nBatch: {batch_number}'
    ax.set_title(title, fontsize=14, fontweight='bold', pad=20)
    
    # Format x-axis
    ax.xaxis.set_major_formatter(mdates.DateFormatter('%H:%M:%S'))
    ax.xaxis.set_major_locator(mdates.AutoDateLocator())
    plt.xticks(rotation=45, ha='right')
    
    # Grid
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)
    
    # Legend
    ax.legend(loc='best', fontsize=11, framealpha=0.9)
    
    # Add statistics text box
    if execution_values or confirmation_values:
        stats_lines = []
        
        if execution_values and any(v > 0 for v in execution_values):
            exec_values_filtered = [v for v in execution_values if v > 0]
            exec_avg = statistics.mean(exec_values_filtered)
            exec_min = min(exec_values_filtered)
            exec_max = max(exec_values_filtered)
            stats_lines.append(f'Execution:    Avg: {exec_avg:.2f} ms  |  Min: {exec_min:.2f} ms  |  Max: {exec_max:.2f} ms')
        
        if confirmation_values and any(v > 0 for v in confirmation_values):
            conf_values_filtered = [v for v in confirmation_values if v > 0]
            conf_avg = statistics.mean(conf_values_filtered)
            conf_min = min(conf_values_filtered)
            conf_max = max(conf_values_filtered)
            stats_lines.append(f'Confirmation: Avg: {conf_avg:.2f} ms  |  Min: {conf_min:.2f} ms  |  Max: {conf_max:.2f} ms')
        
        if stats_lines:
            stats_text = '\n'.join(stats_lines)
            ax.text(0.02, 0.98, stats_text,
                    transform=ax.transAxes,
                    fontsize=10,
                    verticalalignment='top',
                    bbox=dict(boxstyle='round', facecolor='lightblue', alpha=0.8))
    
    plt.tight_layout()
    
    # Save the graph
    ensure_output_dir()
    output_file = os.path.join(OUTPUT_DIR, f'latency_graph_{batch_number if batch_number else "all"}.png')
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"✓ Latency graph saved to: {output_file}")
    
    # Close the plot to free memory
    plt.close()


def select_batch(conn):
    """Helper function to select a batch interactively."""
    batches = get_batch_list(conn)
    
    if not batches:
        print("No batches found in database.")
        return None, []
    
    # Ask user to select a batch or plot all
    print("Available batches:")
    print("  0. All batches (combined)")
    for idx, batch in enumerate(batches, 1):
        print(f"  {idx}. {batch}")
    print()
    
    try:
        choice = input("Select batch number (0 for all, or press Enter for most recent): ").strip()
        
        if choice == "" or choice == str(len(batches)):
            # Most recent batch (default)
            selected_batch = batches[0]
            print(f"\nSelected batch: {selected_batch}")
        elif choice == "0":
            # All batches
            selected_batch = None
            print("\nSelected: All batches")
        elif choice.isdigit() and 1 <= int(choice) <= len(batches):
            selected_batch = batches[int(choice) - 1]
            print(f"\nSelected batch: {selected_batch}")
        else:
            print("Invalid selection. Using most recent batch.")
            selected_batch = batches[0]
        
        return selected_batch, batches
    except (EOFError, KeyboardInterrupt):
        print("\nOperation cancelled.")
        return None, []


def generate_tps_graph(conn, batch_number):
    """Generate TPS graph."""
    print("\n--- TPS Graph ---")
    print("Calculating TPS intervals...")
    submission_tps, confirmation_tps = calculate_tps_intervals(conn, batch_number)
    
    print("Generating graph...")
    plot_tps_graph(submission_tps, confirmation_tps, batch_number)


def generate_latency_graph(conn, batch_number):
    """Generate Latency graph."""
    print("\n--- Latency Graph ---")
    print("Calculating latency intervals...")
    execution_latency, confirmation_latency = calculate_latency_intervals(conn, batch_number)
    
    print("Generating graph...")
    plot_latency_graph(execution_latency, confirmation_latency, batch_number)


def calculate_gas_price_intervals(conn, batch_number=None):
    """
    Calculate average gas prices over 1-second intervals.
    
    Gas price: Price set when transaction was submitted
    Effective gas price: Actual price paid (from receipt)
    
    Returns:
        gas_price_avg: dict of {timestamp: avg_gas_price_gwei}
        effective_gas_price_avg: dict of {timestamp: avg_effective_gas_price_gwei}
    """
    cursor = conn.cursor()
    
    # Query to get transaction and receipt gas prices
    if batch_number:
        query = """
            SELECT submitted_at, gas_price, effective_gas_price, status
            FROM transactions
            WHERE batch_number = ?
            ORDER BY submitted_at
        """
        cursor.execute(query, (batch_number,))
    else:
        query = """
            SELECT submitted_at, gas_price, effective_gas_price, status
            FROM transactions
            ORDER BY submitted_at
        """
        cursor.execute(query)
    
    rows = cursor.fetchall()
    
    if not rows:
        print("No transactions found.")
        return {}, {}
    
    # Group gas prices by time intervals
    gas_price_intervals = defaultdict(list)
    effective_gas_price_intervals = defaultdict(list)
    all_gas_prices = []       # raw values for global stats
    all_effective_prices = [] # raw values for global stats
    
    for row in rows:
        submitted_str, gas_price_str, effective_gas_price_str, status = row
        
        try:
            submitted_dt = parse_timestamp(submitted_str)
            # Round down to nearest interval (INTERVAL_SECONDS)
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            
            # Gas price from transaction (in wei, convert to gwei)
            if gas_price_str:
                try:
                    gas_price_wei = int(gas_price_str)
                    gas_price_gwei = gas_price_wei / 1e9
                    gas_price_intervals[interval_start].append(gas_price_gwei)
                    all_gas_prices.append(gas_price_gwei)
                except (ValueError, TypeError):
                    pass
            
            # Effective gas price from receipt (only for successful transactions)
            if effective_gas_price_str and status == 'success':
                try:
                    effective_gas_price_wei = int(effective_gas_price_str)
                    effective_gas_price_gwei = effective_gas_price_wei / 1e9
                    effective_gas_price_intervals[interval_start].append(effective_gas_price_gwei)
                    all_effective_prices.append(effective_gas_price_gwei)
                except (ValueError, TypeError):
                    pass
        except (ValueError, TypeError):
            continue
    
    # Calculate average gas price for each interval
    gas_price_avg = {ts: statistics.mean(prices) if prices else 0 
                     for ts, prices in gas_price_intervals.items()}
    effective_gas_price_avg = {ts: statistics.mean(prices) if prices else 0 
                               for ts, prices in effective_gas_price_intervals.items()}
    
    return gas_price_avg, effective_gas_price_avg, all_gas_prices, all_effective_prices


def plot_gas_price_graph(gas_price_data, effective_gas_price_data, all_gas_prices=None, all_effective_prices=None, batch_number=None):
    """Create and save the gas price graph."""
    if not gas_price_data and not effective_gas_price_data:
        print("No gas price data to plot.")
        return
    
    # Prepare data for plotting — use submitted gas_price if effective not available
    has_effective = bool(effective_gas_price_data)
    has_submitted = bool(gas_price_data)
    
    all_times = sorted(set(list(gas_price_data.keys()) + list(effective_gas_price_data.keys())))
    
    submitted_values = [gas_price_data.get(t, 0) for t in all_times]
    effective_gas_price_values = [effective_gas_price_data.get(t, 0) for t in all_times]
    
    # Create the plot
    fig, ax = plt.subplots(figsize=(14, 7))
    
    # Plot submitted gas price
    if has_submitted:
        ax.plot(all_times, submitted_values, label='Submitted Gas Price',
                color='#2196F3', linewidth=2, marker='o', markersize=4)
    
    # Plot effective gas price
    if has_effective:
        ax.plot(all_times, effective_gas_price_values, label='Effective Gas Price',
                color='#FF5722', linewidth=2, marker='s', markersize=4)
    
    # Formatting
    ax.set_xlabel('Time', fontsize=12, fontweight='bold')
    ax.set_ylabel('Gas Price (Gwei)', fontsize=12, fontweight='bold')
    
    title = f'Gas Price Over Time ({INTERVAL_SECONDS}s intervals)'
    if batch_number:
        title += f'\nBatch: {batch_number}'
    ax.set_title(title, fontsize=14, fontweight='bold', pad=20)
    
    # Format x-axis
    ax.xaxis.set_major_formatter(mdates.DateFormatter('%H:%M:%S'))
    ax.xaxis.set_major_locator(mdates.AutoDateLocator())
    plt.xticks(rotation=45, ha='right')
    
    # Grid
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)
    
    # Legend
    ax.legend(loc='best', fontsize=11, framealpha=0.9)
    
    # Add statistics text box — use raw per-transaction values for accurate min/max
    stats_lines = []
    raw_submitted = all_gas_prices if all_gas_prices else [v for v in submitted_values if v > 0]
    raw_effective = all_effective_prices if all_effective_prices else [v for v in effective_gas_price_values if v > 0]
    
    if raw_submitted:
        s_avg = statistics.mean(raw_submitted)
        s_min = min(raw_submitted)
        s_max = max(raw_submitted)
        stats_lines.append(f'Submitted:  Avg: {s_avg:.4f} Gwei  |  Min: {s_min:.4f} Gwei  |  Max: {s_max:.4f} Gwei')
    
    if raw_effective:
        e_avg = statistics.mean(raw_effective)
        e_min = min(raw_effective)
        e_max = max(raw_effective)
        stats_lines.append(f'Effective:  Avg: {e_avg:.4f} Gwei  |  Min: {e_min:.4f} Gwei  |  Max: {e_max:.4f} Gwei')
    
    if stats_lines:
        stats_text = '\n'.join(stats_lines)
        ax.text(0.02, 0.98, stats_text,
                transform=ax.transAxes,
                fontsize=10,
                verticalalignment='top',
                bbox=dict(boxstyle='round', facecolor='lightyellow', alpha=0.8))
    
    plt.tight_layout()
    
    # Save the graph
    ensure_output_dir()
    output_file = os.path.join(OUTPUT_DIR, f'gas_price_graph_{batch_number if batch_number else "all"}.png')
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"✓ Gas price graph saved to: {output_file}")
    
    # Close the plot to free memory
    plt.close()


def generate_gas_price_graph(conn, batch_number):
    """Generate Gas Price graph."""
    print("\n--- Gas Price Graph ---")
    print("Calculating gas price intervals...")
    gas_price_data, effective_gas_price_data, all_gas_prices, all_effective_prices = calculate_gas_price_intervals(conn, batch_number)
    
    print("Generating graph...")
    plot_gas_price_graph(gas_price_data, effective_gas_price_data, all_gas_prices, all_effective_prices, batch_number)


def calculate_gas_used_intervals(conn, batch_number=None):
    """
    Calculate total and average gas used over time intervals.
    
    Returns:
        total_gas_used: dict of {timestamp: total_gas}
        avg_gas_used: dict of {timestamp: avg_gas}
    """
    cursor = conn.cursor()
    
    # Query to get gas used from receipts
    if batch_number:
        query = """
            SELECT submitted_at, gas_used, status
            FROM transactions
            WHERE batch_number = ?
            ORDER BY submitted_at
        """
        cursor.execute(query, (batch_number,))
    else:
        query = """
            SELECT submitted_at, gas_used, status
            FROM transactions
            ORDER BY submitted_at
        """
        cursor.execute(query)
    
    rows = cursor.fetchall()
    
    if not rows:
        print("No transactions found.")
        return {}, {}
    
    # Group gas usage by time intervals
    gas_used_intervals = defaultdict(list)
    
    for row in rows:
        submitted_str, gas_used, status = row
        
        try:
            submitted_dt = parse_timestamp(submitted_str)
            # Round down to nearest interval (INTERVAL_SECONDS)
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            
            # Gas used from receipt (only for successful transactions)
            if gas_used is not None and status == 'success':
                try:
                    gas_used_int = int(gas_used)
                    if gas_used_int > 0:
                        gas_used_intervals[interval_start].append(gas_used_int)
                except (ValueError, TypeError):
                    pass
        except (ValueError, TypeError):
            continue
    
    # Calculate total and average gas for each interval
    total_gas = {ts: sum(gas_list) for ts, gas_list in gas_used_intervals.items()}
    avg_gas = {ts: statistics.mean(gas_list) if gas_list else 0 
               for ts, gas_list in gas_used_intervals.items()}
    
    return total_gas, avg_gas


def plot_gas_used_graph(total_gas_data, avg_gas_data, batch_number=None):
    """Create and save the gas used graph."""
    if not total_gas_data:
        print("No gas usage data to plot.")
        return
    
    # Prepare data for plotting
    all_times = sorted(total_gas_data.keys())
    
    total_gas_values = [total_gas_data.get(t, 0) for t in all_times]
    
    # Create the plot
    fig, ax = plt.subplots(figsize=(14, 7))
    
    # Plot total gas used
    ax.plot(all_times, total_gas_values, label='Total Gas Used', 
            color='#2196F3', linewidth=2, marker='o', markersize=4)
    
    # Formatting
    ax.set_xlabel('Time', fontsize=12, fontweight='bold')
    ax.set_ylabel('Total Gas Used', fontsize=12, fontweight='bold')
    
    title = f'Gas Usage Over Time ({INTERVAL_SECONDS}s intervals)'
    if batch_number:
        title += f'\nBatch: {batch_number}'
    ax.set_title(title, fontsize=14, fontweight='bold', pad=20)
    
    # Format x-axis
    ax.xaxis.set_major_formatter(mdates.DateFormatter('%H:%M:%S'))
    ax.xaxis.set_major_locator(mdates.AutoDateLocator())
    plt.xticks(rotation=45, ha='right')
    
    # Grid
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)
    
    # Legend
    ax.legend(loc='best', fontsize=11, framealpha=0.9)
    
    # Add statistics text box
    if total_gas_values and any(v > 0 for v in total_gas_values):
        total_values_filtered = [v for v in total_gas_values if v > 0]
        total_sum = sum(total_values_filtered)
        total_avg = statistics.mean(total_values_filtered)
        total_min = min(total_values_filtered)
        total_max = max(total_values_filtered)
        
        stats_text = f'Sum: {total_sum:,}  |  Avg: {total_avg:,.0f}  |  Min: {total_min:,.0f}  |  Max: {total_max:,}'
        ax.text(0.02, 0.98, stats_text,
                transform=ax.transAxes,
                fontsize=10,
                verticalalignment='top',
                bbox=dict(boxstyle='round', facecolor='lightgreen', alpha=0.8))
    
    plt.tight_layout()
    
    # Save the graph
    ensure_output_dir()
    output_file = os.path.join(OUTPUT_DIR, f'gas_used_graph_{batch_number if batch_number else "all"}.png')
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"✓ Gas used graph saved to: {output_file}")
    
    # Close the plot to free memory
    plt.close()


def generate_gas_used_graph(conn, batch_number):
    """Generate Gas Used graph."""
    print("\n--- Gas Used Graph ---")
    print("Calculating gas usage intervals...")
    total_gas_data, avg_gas_data = calculate_gas_used_intervals(conn, batch_number)
    
    print("Generating graph...")
    plot_gas_used_graph(total_gas_data, avg_gas_data, batch_number)


def main():
    """Main function to generate graphs."""
    print("=== Transaction Metrics Graph Generator ===")
    print()
    
    # Check if database exists
    try:
        conn = sqlite3.connect(DB_PATH)
    except sqlite3.Error as e:
        print(f"Error connecting to database: {e}")
        sys.exit(1)
    
    # Get batch list and select
    selected_batch, batches = select_batch(conn)
    
    if selected_batch is None and not batches:
        conn.close()
        sys.exit(1)
    
    # Ask which graph type to generate
    print("\nSelect graph type:")
    print("  1. TPS Graph (Transactions Per Second)")
    print("  2. Latency Graph (Transaction Timing)")
    print("  3. Gas Price Graph (Effective Gas Price)")
    print("  4. Gas Used Graph (Gas Consumption)")
    print("  5. TPS + Latency Graphs")
    print("  6. All Graphs (TPS + Latency + Gas Price + Gas Used)")
    print()
    
    try:
        graph_choice = input("Enter choice (1-6, or press Enter for all): ").strip()
        
        if graph_choice == "" or graph_choice == "6":
            print("\nGenerating all graphs...")
            generate_tps_graph(conn, selected_batch)
            generate_latency_graph(conn, selected_batch)
            generate_gas_price_graph(conn, selected_batch)
            generate_gas_used_graph(conn, selected_batch)
        elif graph_choice == "1":
            generate_tps_graph(conn, selected_batch)
        elif graph_choice == "2":
            generate_latency_graph(conn, selected_batch)
        elif graph_choice == "3":
            generate_gas_price_graph(conn, selected_batch)
        elif graph_choice == "4":
            generate_gas_used_graph(conn, selected_batch)
        elif graph_choice == "5":
            print("\nGenerating TPS and Latency graphs...")
            generate_tps_graph(conn, selected_batch)
            generate_latency_graph(conn, selected_batch)
        else:
            print("Invalid choice. Generating all graphs...")
            generate_tps_graph(conn, selected_batch)
            generate_latency_graph(conn, selected_batch)
            generate_gas_price_graph(conn, selected_batch)
            generate_gas_used_graph(conn, selected_batch)
    except (EOFError, KeyboardInterrupt):
        print("\nOperation cancelled.")
        conn.close()
        sys.exit(0)
    
    # Close database connection
    conn.close()
    
    print("\nDone! All graphs saved in the 'images/' directory.")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\nOperation cancelled by user.")
        sys.exit(0)
