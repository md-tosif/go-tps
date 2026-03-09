#!/usr/bin/env python3
"""
Multi-Database Analysis Script for go-tps
Generates CSV with comprehensive statistics from multiple SQLite databases
"""

import sqlite3
import csv
import sys
from pathlib import Path
from datetime import datetime
import argparse


def analyze_database(db_path):
    """
    Analyze a single database and return comprehensive statistics
    
    Args:
        db_path: Path to SQLite database file
        
    Returns:
        Dictionary containing all statistics
    """
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    stats = {
        'database_name': Path(db_path).name,
        'db_path': db_path
    }
    
    # ===== BASIC COUNTS =====
    # Total transactions and status breakdown
    cursor.execute("""
        SELECT 
            COUNT(*) as total_txns,
            SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
            SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
            SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending
        FROM transactions
    """)
    row = cursor.fetchone()
    stats['total_transactions'] = row[0] or 0
    stats['successful_transactions'] = row[1] or 0
    stats['failed_transactions'] = row[2] or 0
    stats['pending_transactions'] = row[3] or 0
    
    # Success rate percentage
    if stats['total_transactions'] > 0:
        stats['success_rate_percent'] = round((stats['successful_transactions'] / stats['total_transactions']) * 100, 2)
        stats['failure_rate_percent'] = round((stats['failed_transactions'] / stats['total_transactions']) * 100, 2)
    else:
        stats['success_rate_percent'] = 0
        stats['failure_rate_percent'] = 0
    
    # ===== SUBMISSION LATENCY =====
    # Time between first and last submission (execution_time is confirmation latency)
    cursor.execute("""
        SELECT 
            MIN(submitted_at) as first_submission,
            MAX(submitted_at) as last_submission,
            (JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400 as submission_window_seconds
        FROM transactions
        WHERE submitted_at IS NOT NULL
    """)
    row = cursor.fetchone()
    stats['first_submission_time'] = row[0]
    stats['last_submission_time'] = row[1]
    stats['submission_window_seconds'] = round(row[2], 3) if row[2] else 0
    
    # ===== CONFIRMATION LATENCY (execution_time field) =====
    # execution_time is in milliseconds - time from submission to confirmation
    cursor.execute("""
        SELECT 
            MIN(execution_time) as min_confirmation_latency_ms,
            MAX(execution_time) as max_confirmation_latency_ms,
            AVG(execution_time) as avg_confirmation_latency_ms
        FROM transactions
        WHERE execution_time IS NOT NULL AND execution_time > 0
    """)
    row = cursor.fetchone()
    stats['min_confirmation_latency_ms'] = round(row[0], 3) if row[0] else 0
    stats['max_confirmation_latency_ms'] = round(row[1], 3) if row[1] else 0
    stats['avg_confirmation_latency_ms'] = round(row[2], 3) if row[2] else 0
    
    # Convert to seconds for better readability
    stats['min_confirmation_latency_sec'] = round(stats['min_confirmation_latency_ms'] / 1000, 3)
    stats['max_confirmation_latency_sec'] = round(stats['max_confirmation_latency_ms'] / 1000, 3)
    stats['avg_confirmation_latency_sec'] = round(stats['avg_confirmation_latency_ms'] / 1000, 3)
    
    # ===== TPS METRICS =====
    # TPS based on submission window
    if stats['submission_window_seconds'] > 0:
        stats['tps_submission'] = round(stats['total_transactions'] / stats['submission_window_seconds'], 2)
    else:
        stats['tps_submission'] = 0
    
    # TPS based on confirmation window (only successful transactions)
    cursor.execute("""
        SELECT 
            COUNT(*) as confirmed_count,
            MIN(confirmed_at) as first_confirmed,
            MAX(confirmed_at) as last_confirmed,
            (JULIANDAY(MAX(confirmed_at)) - JULIANDAY(MIN(confirmed_at))) * 86400 as confirmation_window_seconds
        FROM transactions
        WHERE status = 'success' AND confirmed_at IS NOT NULL
    """)
    row = cursor.fetchone()
    confirmed_count = row[0] or 0
    confirmation_window = row[3] if row[3] else 0
    
    if confirmation_window > 0:
        stats['tps_confirmation'] = round(confirmed_count / confirmation_window, 2)
    else:
        stats['tps_confirmation'] = 0
    
    stats['confirmation_window_seconds'] = round(confirmation_window, 3)
    
    # TPS statistics (min, max, avg across batches)
    cursor.execute("""
        SELECT 
            batch_number,
            COUNT(*) as tx_count,
            (JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400 as duration_sec
        FROM transactions
        GROUP BY batch_number
        HAVING duration_sec > 0
    """)
    
    batch_tps_values = []
    for row in cursor.fetchall():
        tps = row[1] / row[2]  # tx_count / duration_sec
        batch_tps_values.append(tps)
    
    if batch_tps_values:
        stats['min_tps_batch'] = round(min(batch_tps_values), 2)
        stats['max_tps_batch'] = round(max(batch_tps_values), 2)
        stats['avg_tps_batch'] = round(sum(batch_tps_values) / len(batch_tps_values), 2)
    else:
        stats['min_tps_batch'] = 0
        stats['max_tps_batch'] = 0
        stats['avg_tps_batch'] = 0
    
    # ===== GAS PRICE (in wei) =====
    cursor.execute("""
        SELECT 
            MIN(CAST(gas_price AS INTEGER)) as min_gas_price,
            MAX(CAST(gas_price AS INTEGER)) as max_gas_price,
            AVG(CAST(gas_price AS INTEGER)) as avg_gas_price
        FROM transactions
        WHERE gas_price IS NOT NULL AND gas_price != ''
    """)
    row = cursor.fetchone()
    stats['min_gas_price_wei'] = row[0] if row[0] else 0
    stats['max_gas_price_wei'] = row[1] if row[1] else 0
    stats['avg_gas_price_wei'] = round(row[2], 0) if row[2] else 0
    
    # Convert to Gwei (1 Gwei = 10^9 wei)
    stats['min_gas_price_gwei'] = round(stats['min_gas_price_wei'] / 1e9, 2)
    stats['max_gas_price_gwei'] = round(stats['max_gas_price_wei'] / 1e9, 2)
    stats['avg_gas_price_gwei'] = round(stats['avg_gas_price_wei'] / 1e9, 2)
    
    # ===== EFFECTIVE GAS PRICE (in wei) =====
    cursor.execute("""
        SELECT 
            MIN(CAST(effective_gas_price AS INTEGER)) as min_effective_gas_price,
            MAX(CAST(effective_gas_price AS INTEGER)) as max_effective_gas_price,
            AVG(CAST(effective_gas_price AS INTEGER)) as avg_effective_gas_price
        FROM transactions
        WHERE effective_gas_price IS NOT NULL AND effective_gas_price != ''
    """)
    row = cursor.fetchone()
    stats['min_effective_gas_price_wei'] = row[0] if row[0] else 0
    stats['max_effective_gas_price_wei'] = row[1] if row[1] else 0
    stats['avg_effective_gas_price_wei'] = round(row[2], 0) if row[2] else 0
    
    # Convert to Gwei
    stats['min_effective_gas_price_gwei'] = round(stats['min_effective_gas_price_wei'] / 1e9, 2)
    stats['max_effective_gas_price_gwei'] = round(stats['max_effective_gas_price_wei'] / 1e9, 2)
    stats['avg_effective_gas_price_gwei'] = round(stats['avg_effective_gas_price_wei'] / 1e9, 2)
    
    # ===== GAS USED (gas units consumed) =====
    cursor.execute("""
        SELECT 
            MIN(gas_used) as min_gas_used,
            MAX(gas_used) as max_gas_used,
            AVG(gas_used) as avg_gas_used,
            SUM(gas_used) as total_gas_used
        FROM transactions
        WHERE gas_used IS NOT NULL AND gas_used > 0
    """)
    row = cursor.fetchone()
    stats['min_gas_used'] = row[0] if row[0] else 0
    stats['max_gas_used'] = row[1] if row[1] else 0
    stats['avg_gas_used'] = round(row[2], 2) if row[2] else 0
    stats['total_gas_used'] = row[3] if row[3] else 0
    
    # ===== GAS LIMIT =====
    cursor.execute("""
        SELECT 
            MIN(gas_limit) as min_gas_limit,
            MAX(gas_limit) as max_gas_limit,
            AVG(gas_limit) as avg_gas_limit
        FROM transactions
        WHERE gas_limit IS NOT NULL
    """)
    row = cursor.fetchone()
    stats['min_gas_limit'] = row[0] if row[0] else 0
    stats['max_gas_limit'] = row[1] if row[1] else 0
    stats['avg_gas_limit'] = round(row[2], 2) if row[2] else 0
    
    # ===== TOTAL TRANSACTION COST =====
    # Calculate total cost in wei (gas_used * effective_gas_price)
    cursor.execute("""
        SELECT 
            SUM(gas_used * CAST(effective_gas_price AS INTEGER)) as total_cost_wei
        FROM transactions
        WHERE gas_used IS NOT NULL AND effective_gas_price IS NOT NULL 
            AND gas_used > 0 AND effective_gas_price != ''
    """)
    row = cursor.fetchone()
    total_cost_wei = row[0] if row[0] else 0
    stats['total_cost_wei'] = total_cost_wei
    stats['total_cost_eth'] = round(total_cost_wei / 1e18, 6)  # Convert wei to ETH
    
    # Average cost per transaction
    if stats['successful_transactions'] > 0:
        stats['avg_cost_per_tx_wei'] = round(total_cost_wei / stats['successful_transactions'], 0)
        stats['avg_cost_per_tx_eth'] = round(stats['avg_cost_per_tx_wei'] / 1e18, 9)
    else:
        stats['avg_cost_per_tx_wei'] = 0
        stats['avg_cost_per_tx_eth'] = 0
    
    # ===== BATCH INFORMATION =====
    cursor.execute("SELECT COUNT(DISTINCT batch_number) FROM transactions")
    stats['total_batches'] = cursor.fetchone()[0] or 0
    
    # ===== WALLET INFORMATION =====
    cursor.execute("SELECT COUNT(DISTINCT wallet_address) FROM transactions")
    stats['unique_wallets'] = cursor.fetchone()[0] or 0
    
    conn.close()
    
    return stats


def generate_csv(databases, output_file='db_summary.csv'):
    """
    Generate CSV file with analysis of multiple databases
    
    Args:
        databases: List of database file paths
        output_file: Output CSV filename
    """
    all_stats = []
    
    print(f"Analyzing {len(databases)} database(s)...")
    
    for db_path in databases:
        if not Path(db_path).exists():
            print(f"Warning: Database not found: {db_path}")
            continue
        
        print(f"  Analyzing: {db_path}")
        try:
            stats = analyze_database(db_path)
            all_stats.append(stats)
        except Exception as e:
            print(f"  Error analyzing {db_path}: {e}")
            continue
    
    if not all_stats:
        print("No databases were successfully analyzed!")
        return
    
    # Define column order for CSV
    columns = [
        # Basic Info
        'database_name',
        'db_path',
        
        # Transaction Counts
        'total_transactions',
        'successful_transactions',
        'failed_transactions',
        'pending_transactions',
        'success_rate_percent',
        'failure_rate_percent',
        
        # Submission Metrics
        'submission_window_seconds',
        'first_submission_time',
        'last_submission_time',
        
        # Confirmation Latency (milliseconds)
        'min_confirmation_latency_ms',
        'max_confirmation_latency_ms',
        'avg_confirmation_latency_ms',
        
        # Confirmation Latency (seconds)
        'min_confirmation_latency_sec',
        'max_confirmation_latency_sec',
        'avg_confirmation_latency_sec',
        
        # TPS Metrics
        'tps_submission',
        'tps_confirmation',
        'confirmation_window_seconds',
        'min_tps_batch',
        'max_tps_batch',
        'avg_tps_batch',
        
        # Gas Price (Wei)
        'min_gas_price_wei',
        'max_gas_price_wei',
        'avg_gas_price_wei',
        
        # Gas Price (Gwei)
        'min_gas_price_gwei',
        'max_gas_price_gwei',
        'avg_gas_price_gwei',
        
        # Effective Gas Price (Wei)
        'min_effective_gas_price_wei',
        'max_effective_gas_price_wei',
        'avg_effective_gas_price_wei',
        
        # Effective Gas Price (Gwei)
        'min_effective_gas_price_gwei',
        'max_effective_gas_price_gwei',
        'avg_effective_gas_price_gwei',
        
        # Gas Used
        'min_gas_used',
        'max_gas_used',
        'avg_gas_used',
        'total_gas_used',
        
        # Gas Limit
        'min_gas_limit',
        'max_gas_limit',
        'avg_gas_limit',
        
        # Transaction Costs
        'total_cost_wei',
        'total_cost_eth',
        'avg_cost_per_tx_wei',
        'avg_cost_per_tx_eth',
        
        # Additional Info
        'total_batches',
        'unique_wallets',
    ]
    
    # Write CSV
    with open(output_file, 'w', newline='') as csvfile:
        writer = csv.DictWriter(csvfile, fieldnames=columns)
        writer.writeheader()
        writer.writerows(all_stats)
    
    print(f"\n✓ CSV generated successfully: {output_file}")
    print(f"  Databases analyzed: {len(all_stats)}")
    print(f"  Columns: {len(columns)}")
    
    # Print summary
    print("\n" + "="*80)
    print("SUMMARY")
    print("="*80)
    for stats in all_stats:
        print(f"\n{stats['database_name']}:")
        print(f"  Total Transactions: {stats['total_transactions']:,}")
        print(f"  Success Rate: {stats['success_rate_percent']}%")
        print(f"  TPS (Submission): {stats['tps_submission']}")
        print(f"  TPS (Confirmation): {stats['tps_confirmation']}")
        print(f"  Avg Confirmation Latency: {stats['avg_confirmation_latency_ms']} ms")
        print(f"  Avg Gas Used: {stats['avg_gas_used']}")
        print(f"  Total Cost: {stats['total_cost_eth']} ETH")


def main():
    parser = argparse.ArgumentParser(
        description='Analyze multiple go-tps SQLite databases and generate CSV summary',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Analyze specific databases
  python3 analyze_multi_db.py load-1.db load-2.db
  
  # Analyze all .db files in current directory
  python3 analyze_multi_db.py *.db
  
  # Specify custom output file
  python3 analyze_multi_db.py -o results.csv load-1.db load-2.db transactions.db
        """
    )
    
    parser.add_argument(
        'databases',
        nargs='+',
        help='SQLite database files to analyze'
    )
    
    parser.add_argument(
        '-o', '--output',
        default='db_summary.csv',
        help='Output CSV filename (default: db_summary.csv)'
    )
    
    args = parser.parse_args()
    
    generate_csv(args.databases, args.output)


if __name__ == '__main__':
    main()
