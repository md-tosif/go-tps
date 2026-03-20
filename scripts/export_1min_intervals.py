#!/usr/bin/env python3
"""
1-Minute Interval CSV Export Tool for go-tps
Exports transaction metrics aggregated by 1-minute intervals to CSV.

Metrics exported:
- Gas used in 1-minute intervals
- Confirmation TPS in that 1 minute
- Confirmation latency (average) in that 1 minute
- Submission TPS in 1 minute
- Success rate in 1 minute
- Failure rate in 1 minute
"""

import sqlite3
import sys
import os
import csv
import argparse
from datetime import datetime


def get_1min_intervals_data(db_path, batch_number=None):
    """
    Get transaction data aggregated by 1-minute intervals using database queries.
    Returns a list of metrics dictionaries for each minute interval.
    """
    try:
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()
        
        # Build WHERE clause for optional batch filter
        where_clause = ""
        params = []
        if batch_number:
            where_clause = "WHERE batch_number = ?"
            params.append(batch_number)
        
        # Query for submission metrics (grouped by submission minute)
        submission_query = f"""
        SELECT 
            strftime('%Y-%m-%d %H:%M:00', submitted_at) as minute_interval,
            COUNT(*) as submitted_count,
            (SUM(CASE WHEN gas_used > 0 THEN gas_used ELSE gas_limit END) / 60.0) as avg_gas_used_per_second,
            SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
            SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failure_count,
            AVG(execution_time) as avg_execution_time_ms
        FROM transactions 
        {where_clause}
        GROUP BY strftime('%Y-%m-%d %H:%M:00', submitted_at)
        ORDER BY minute_interval
        """
        
        cursor.execute(submission_query, params)
        submission_data = cursor.fetchall()
        
        # Query for confirmation metrics (grouped by confirmation minute)
        confirmation_where = "WHERE confirmed_at IS NOT NULL"
        if batch_number:
            confirmation_where = "WHERE batch_number = ? AND confirmed_at IS NOT NULL"
            
        confirmation_query = f"""
        SELECT 
            strftime('%Y-%m-%d %H:%M:00', confirmed_at) as minute_interval,
            COUNT(*) as confirmed_count,
            AVG((JULIANDAY(confirmed_at) - JULIANDAY(submitted_at)) * 86400 * 1000) as avg_latency_ms
        FROM transactions 
        {confirmation_where}
        GROUP BY strftime('%Y-%m-%d %H:%M:00', confirmed_at)
        ORDER BY minute_interval
        """
        
        cursor.execute(confirmation_query, params)
        confirmation_data = cursor.fetchall()
        
        conn.close()
        
        if not submission_data:
            print("No transactions found in database.")
            return []
        
        print(f"Processing {len(submission_data)} 1-minute intervals...")
        
        # Convert to dictionaries for easier lookup
        submission_dict = {row['minute_interval']: dict(row) for row in submission_data}
        confirmation_dict = {row['minute_interval']: dict(row) for row in confirmation_data}
        
        # Combine submission and confirmation data
        intervals = []
        for minute, sub_data in submission_dict.items():
            conf_data = confirmation_dict.get(minute, {})
            
            intervals.append({
                'minute_interval': minute,
                'submitted_count': sub_data['submitted_count'] or 0,
                'confirmed_count': conf_data.get('confirmed_count', 0) or 0,
                'avg_gas_used_per_second': sub_data.get('avg_gas_used_per_second', 0) or 0,
                'success_count': sub_data['success_count'] or 0,
                'failure_count': sub_data['failure_count'] or 0,
                'avg_latency_ms': conf_data.get('avg_latency_ms', 0) or 0,
                'avg_execution_time_ms': sub_data.get('avg_execution_time_ms', 0) or 0
            })
        
        return intervals
        
    except sqlite3.Error as e:
        print(f"Database error: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"Error processing data: {e}")
        sys.exit(1)


def calculate_metrics(intervals_data):
    """Calculate final metrics from database-aggregated data."""
    metrics = []
    
    for data in intervals_data:
        # Basic counts (already from database)
        submitted_count = data['submitted_count']
        confirmed_count = data['confirmed_count']
        success_count = data['success_count']
        failure_count = data['failure_count']
        avg_gas_used_per_second = data['avg_gas_used_per_second']
        avg_latency_ms = data['avg_latency_ms']
        avg_execution_time_ms = data['avg_execution_time_ms']
        
        # TPS calculations (transactions per second in this minute)
        submission_tps = submitted_count / 60.0
        confirmation_tps = confirmed_count / 60.0
        
        # Success/failure rates (as percentages)
        success_rate = (success_count / submitted_count * 100) if submitted_count > 0 else 0
        failure_rate = (failure_count / submitted_count * 100) if submitted_count > 0 else 0
        
        # Parse timestamp from string
        timestamp = datetime.strptime(data['minute_interval'], '%Y-%m-%d %H:%M:%S')
        
        metrics.append({
            'timestamp': timestamp,
            'submitted_count': submitted_count,
            'confirmed_count': confirmed_count,
            'avg_gas_used_per_second': round(avg_gas_used_per_second, 2),
            'submission_tps': round(submission_tps, 3),
            'confirmation_tps': round(confirmation_tps, 3),
            'avg_confirmation_latency_ms': round(avg_latency_ms, 2),
            'avg_execution_time_ms': round(avg_execution_time_ms, 2),
            'success_count': success_count,
            'failure_count': failure_count,
            'success_rate_percent': round(success_rate, 2),
            'failure_rate_percent': round(failure_rate, 2)
        })
    
    return metrics


def export_to_csv(metrics, output_file):
    """Export metrics to CSV file."""
    if not metrics:
        print("No metrics to export.")
        return
    
    fieldnames = [
        'timestamp',
        'submitted_count',
        'confirmed_count', 
        'avg_gas_used_per_second',
        'submission_tps',
        'confirmation_tps',
        'avg_confirmation_latency_ms',
        'avg_execution_time_ms',
        'success_count',
        'failure_count',
        'success_rate_percent',
        'failure_rate_percent'
    ]
    
    try:
        with open(output_file, 'w', newline='', encoding='utf-8') as csvfile:
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()
            
            for metric in metrics:
                # Format timestamp for CSV
                metric_copy = metric.copy()
                metric_copy['timestamp'] = metric['timestamp'].strftime('%Y-%m-%d %H:%M:%S')
                writer.writerow(metric_copy)
        
        print(f"✓ Metrics exported to {output_file}")
        print(f"✓ {len(metrics)} 1-minute intervals exported")
        
    except Exception as e:
        print(f"Error writing CSV: {e}")
        sys.exit(1)


def print_summary(metrics):
    """Print summary statistics."""
    if not metrics:
        return
        
    print("\n=== SUMMARY ===")
    total_submitted = sum(m['submitted_count'] for m in metrics)
    total_confirmed = sum(m['confirmed_count'] for m in metrics)
    avg_gas_per_second = sum(m['avg_gas_used_per_second'] for m in metrics) / len(metrics)
    
    avg_submission_tps = sum(m['submission_tps'] for m in metrics) / len(metrics)
    avg_confirmation_tps = sum(m['confirmation_tps'] for m in metrics) / len(metrics)
    
    overall_success_rate = (
        sum(m['success_count'] for m in metrics) / total_submitted * 100
        if total_submitted > 0 else 0
    )
    
    print(f"Time periods: {len(metrics)} minutes")
    print(f"Total transactions submitted: {total_submitted}")
    print(f"Total transactions confirmed: {total_confirmed}")
    print(f"Average gas used per second: {avg_gas_per_second:,.2f}")
    print(f"Average submission TPS: {avg_submission_tps:.3f}")
    print(f"Average confirmation TPS: {avg_confirmation_tps:.3f}")
    print(f"Overall success rate: {overall_success_rate:.2f}%")


def get_available_batches(db_path):
    """Get list of available batch numbers."""
    try:
        conn = sqlite3.connect(db_path)
        cursor = conn.cursor()
        cursor.execute("SELECT DISTINCT batch_number FROM transactions ORDER BY batch_number DESC")
        batches = [row[0] for row in cursor.fetchall()]
        conn.close()
        return batches
    except sqlite3.Error as e:
        print(f"Database error: {e}")
        return []


def main():
    parser = argparse.ArgumentParser(
        description='Export go-tps transaction metrics in 1-minute intervals to CSV',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 export_1min_intervals.py
  python3 export_1min_intervals.py --db custom.db
  python3 export_1min_intervals.py --batch batch-20260316-120000
  python3 export_1min_intervals.py --output-file my_metrics.csv
  python3 export_1min_intervals.py --list-batches
        """
    )
    
    parser.add_argument('--db', '--database', 
                       default='./transactions.db',
                       help='Path to SQLite database file (default: ./transactions.db)')
    
    parser.add_argument('--batch', '--batch-number',
                       help='Filter by specific batch number (optional)')
    
    parser.add_argument('--output-file', '-o',
                       help='Output CSV filename (default: auto-generated based on timestamp)')
    
    parser.add_argument('--list-batches', action='store_true',
                       help='List available batch numbers and exit')
    
    parser.add_argument('--quiet', '-q', action='store_true',
                       help='Suppress summary output')
    
    args = parser.parse_args()
    
    # Check if database exists
    if not os.path.exists(args.db):
        print(f"Error: Database file '{args.db}' not found.")
        print("Run go-tps first to generate transaction data.")
        sys.exit(1)
    
    # List batches if requested
    if args.list_batches:
        batches = get_available_batches(args.db)
        if batches:
            print("Available batches:")
            for batch in batches:
                print(f"  {batch}")
        else:
            print("No batches found in database.")
        sys.exit(0)
    
    # Generate output filename if not specified
    if not args.output_file:
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        batch_suffix = f"_{args.batch}" if args.batch else ""
        args.output_file = f"1min_intervals{batch_suffix}_{timestamp}.csv"
    
    print(f"Database: {args.db}")
    if args.batch:
        print(f"Batch filter: {args.batch}")
    print(f"Output file: {args.output_file}")
    print()
    
    # Get data and calculate metrics
    intervals_data = get_1min_intervals_data(args.db, args.batch)
    if not intervals_data:
        print("No data found to process.")
        sys.exit(1)
    
    metrics = calculate_metrics(intervals_data)
    
    # Export to CSV
    export_to_csv(metrics, args.output_file)
    
    # Print summary unless quiet mode
    if not args.quiet:
        print_summary(metrics)


if __name__ == '__main__':
    main()