#!/bin/bash
# Quick usage examples for analyze_multi_db.py

echo "================================"
echo "Multi-Database Analysis Examples"
echo "================================"
echo ""

# Example 1: Analyze specific databases
echo "1. Analyze specific databases:"
echo "   python3 analyze_multi_db.py load-1.db load-2.db transactions.db"
echo ""

# Example 2: Analyze all databases
echo "2. Analyze all .db files in current directory:"
echo "   python3 analyze_multi_db.py *.db"
echo ""

# Example 3: Custom output
echo "3. Specify custom output filename:"
echo "   python3 analyze_multi_db.py -o my_report.csv load-1.db load-2.db"
echo ""

# Example 4: View CSV in terminal
echo "4. View CSV in terminal (formatted):"
echo "   column -s, -t db_summary.csv | less -S"
echo ""

# Example 5: View specific columns
echo "5. View specific metrics only:"
echo "   csvcut -c database_name,total_transactions,tps_submission,success_rate_percent db_summary.csv | column -t -s,"
echo ""

# Example 6: Open in spreadsheet
echo "6. Open in LibreOffice Calc:"
echo "   libreoffice db_summary.csv"
echo ""

# Example 7: Get help
echo "7. Show help:"
echo "   python3 analyze_multi_db.py --help"
echo ""

echo "================================"
echo "Current Status"
echo "================================"
echo ""

if [ -f "db_summary.csv" ]; then
    echo "✓ CSV file exists: db_summary.csv"
    echo "  Rows: $(wc -l < db_summary.csv)"
    echo "  Columns: $(head -1 db_summary.csv | tr ',' '\n' | wc -l)"
    echo ""
    echo "Databases analyzed:"
    tail -n +2 db_summary.csv | cut -d, -f1
else
    echo "✗ CSV file not found. Run the analysis first:"
    echo "  python3 analyze_multi_db.py *.db"
fi

echo ""
echo "================================"
echo "For detailed column descriptions, see:"
echo "  DB_ANALYSIS_README.md"
echo "================================"
