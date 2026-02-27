#!/usr/bin/env python3
"""
Transaction Metrics Graph Generator
Generates graphs for TPS (Transactions Per Second) and Latency metrics
over 1-second intervals from the transactions database.
"""

import sqlite3
import sys
import os
from datetime import datetime, timedelta
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
from collections import defaultdict
import statistics

DB_PATH = "./transactions.db"
INTERVAL_SECONDS = 1
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
            submitted_dt = datetime.fromisoformat(submitted_str)
            # Round down to nearest 1-second interval
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            submission_intervals[interval_start] += 1
        except (ValueError, TypeError):
            continue
        
        # Parse confirmation time (if available and successful)
        if confirmed_str and status == 'success':
            try:
                confirmed_dt = datetime.fromisoformat(confirmed_str)
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
    Calculate average latency for both submission and confirmation over 1-second intervals.
    
    Submission latency: Time taken to submit the transaction (execution_time in DB)
    Confirmation latency: Time from submission to confirmation
    
    Returns:
        submission_latency: dict of {timestamp: avg_latency_ms}
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
    submission_intervals = defaultdict(list)
    confirmation_intervals = defaultdict(list)
    
    for row in rows:
        submitted_str, confirmed_str, exec_time, status = row
        
        try:
            submitted_dt = datetime.fromisoformat(submitted_str)
            # Round down to nearest interval (INTERVAL_SECONDS)
            interval_start = submitted_dt.replace(microsecond=0)
            interval_start = interval_start - timedelta(seconds=interval_start.second % INTERVAL_SECONDS)
            
            # Submission latency (from execution_time in DB - time to submit)
            if exec_time is not None and exec_time > 0:
                submission_intervals[interval_start].append(exec_time)
            
            # Confirmation latency (time from submission to confirmation)
            if confirmed_str and status == 'success':
                try:
                    confirmed_dt = datetime.fromisoformat(confirmed_str)
                    confirmation_latency_ms = (confirmed_dt - submitted_dt).total_seconds() * 1000
                    if confirmation_latency_ms > 0:
                        confirmation_intervals[interval_start].append(confirmation_latency_ms)
                except (ValueError, TypeError):
                    continue
        except (ValueError, TypeError):
            continue
    
    # Calculate average latency for each interval
    submission_avg = {ts: statistics.mean(latencies) if latencies else 0 
                      for ts, latencies in submission_intervals.items()}
    confirmation_avg = {ts: statistics.mean(latencies) if latencies else 0 
                        for ts, latencies in confirmation_intervals.items()}
    
    return submission_avg, confirmation_avg


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
        sub_avg = sum(submission_values) / len(submission_values)
        sub_max = max(submission_values)
        conf_avg = sum(confirmation_values) / len(confirmation_values) if confirmation_values else 0
        conf_max = max(confirmation_values) if confirmation_values else 0
        
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


def plot_latency_graph(submission_latency, confirmation_latency, batch_number=None):
    """Create and save the latency graph."""
    if not submission_latency and not confirmation_latency:
        print("No data to plot.")
        return
    
    # Prepare data for plotting
    all_times = sorted(set(list(submission_latency.keys()) + list(confirmation_latency.keys())))
    
    submission_values = [submission_latency.get(t, 0) for t in all_times]
    confirmation_values = [confirmation_latency.get(t, 0) for t in all_times]
    
    # Create the plot
    fig, ax = plt.subplots(figsize=(14, 7))
    
    # Plot both lines
    ax.plot(all_times, submission_values, label='Submission Latency', 
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
    if submission_values or confirmation_values:
        stats_lines = []
        
        if submission_values and any(v > 0 for v in submission_values):
            sub_values_filtered = [v for v in submission_values if v > 0]
            sub_avg = statistics.mean(sub_values_filtered)
            sub_min = min(sub_values_filtered)
            sub_max = max(sub_values_filtered)
            stats_lines.append(f'Submission:  Avg: {sub_avg:.2f} ms  |  Min: {sub_min:.2f} ms  |  Max: {sub_max:.2f} ms')
        
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
    submission_latency, confirmation_latency = calculate_latency_intervals(conn, batch_number)
    
    print("Generating graph...")
    plot_latency_graph(submission_latency, confirmation_latency, batch_number)


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
    print("  3. Both Graphs")
    print()
    
    try:
        graph_choice = input("Enter choice (1-3, or press Enter for both): ").strip()
        
        if graph_choice == "" or graph_choice == "3":
            print("\nGenerating both graphs...")
            generate_tps_graph(conn, selected_batch)
            generate_latency_graph(conn, selected_batch)
        elif graph_choice == "1":
            generate_tps_graph(conn, selected_batch)
        elif graph_choice == "2":
            generate_latency_graph(conn, selected_batch)
        else:
            print("Invalid choice. Generating both graphs...")
            generate_tps_graph(conn, selected_batch)
            generate_latency_graph(conn, selected_batch)
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
