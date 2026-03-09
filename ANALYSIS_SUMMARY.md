# Multi-Database Analysis - Summary

## ✅ What Was Created

### 1. **analyze_multi_db.py** - Main Analysis Script
- Comprehensive Python script to analyze multiple SQLite databases
- Extracts 48 different metrics from each database
- Generates CSV output with all statistics

### 2. **db_summary.csv** - Generated CSV Report
- Contains analysis of 3 databases: load-1.db, load-2.db, transactions.db
- 48 columns covering all requested metrics
- Ready to open in Excel, LibreOffice, or any CSV viewer

### 3. **DB_ANALYSIS_README.md** - Documentation
- Detailed explanation of all 48 columns in the CSV
- Usage examples and tips
- Troubleshooting guide

### 4. **usage_examples.sh** - Quick Reference
- Shell script with usage examples
- Shows current analysis status
- Quick commands for viewing and working with CSV

---

## 📊 Metrics Included (All Your Requirements)

### ✅ Submission & Confirmation Latency
- **Min/Max/Avg confirmation latency** (in milliseconds and seconds)
- **Submission window** (time between first and last submission)
- **Confirmation window** (time between first and last confirmation)

### ✅ TPS (Transactions Per Second)
- **TPS submission** - based on submission time window
- **TPS confirmation** - based on confirmation time window
- **Min/Max/Avg TPS per batch** - variation across batches

### ✅ Gas Used Per Transaction
- **Min/Max/Avg gas used** per transaction
- **Total gas used** across all transactions

### ✅ Success & Failure Counts
- **Total transactions**
- **Successful transactions**
- **Failed transactions**
- **Pending transactions**
- **Success rate %**
- **Failure rate %**

### ✅ Gas Price
- **Min/Max/Avg gas price** (in Wei and Gwei)
- **Min/Max/Avg effective gas price** (in Wei and Gwei)

### ✅ Gas Limit
- **Min/Max/Avg gas limit** per transaction

### ✅ Bonus Metrics
- **Total cost** (in Wei and ETH)
- **Average cost per transaction**
- **Total batches** in database
- **Unique wallets** used

---

## 🚀 Quick Start

### Run Analysis
```bash
# Analyze all databases
python3 analyze_multi_db.py load-1.db load-2.db transactions.db

# Analyze all .db files in current directory
python3 analyze_multi_db.py *.db

# Custom output filename
python3 analyze_multi_db.py -o my_report.csv load-1.db load-2.db
```

### View Results

**In Terminal:**
```bash
# Formatted view
column -s, -t db_summary.csv | less -S

# Show specific columns
csvcut -c database_name,total_transactions,tps_submission,success_rate_percent db_summary.csv | column -t -s,
```

**In Spreadsheet:**
```bash
# LibreOffice
libreoffice db_summary.csv

# Excel (if available)
excel db_summary.csv
```

**Get Help:**
```bash
python3 analyze_multi_db.py --help
./usage_examples.sh
```

---

## 📈 Key Findings from Current Analysis

### load-1.db
- 179,900 transactions (100% success rate)
- TPS: 99.97 (submission), 99.94 (confirmation)
- Avg confirmation latency: 9.045 ms
- Total cost: 0.003779 ETH

### load-2.db
- 539,700 transactions (100% success rate)
- TPS: 299.78 (submission), 299.67 (confirmation)
- Avg confirmation latency: 16.791 ms
- Total cost: 0.011347 ETH
- **3x more transactions** than load-1.db
- **3x higher TPS** than load-1.db

### transactions.db
- 5,717 transactions (0% success, 54% failed, 46% pending)
- TPS: 3.43 (submission)
- Avg confirmation latency: 2817.421 ms (very high!)
- Total cost: 0.0 ETH (no successful transactions)
- **Performance issue detected** - most transactions failed or pending

---

## 💡 Insights

### Performance Comparison
1. **load-2.db has best throughput**: 299.78 TPS vs 99.97 TPS (load-1)
2. **load-1.db has lower latency**: 9.045 ms vs 16.791 ms (load-2)
3. **transactions.db has issues**: 0% success rate suggests network/config problems

### Batch Variation
- **load-1.db**: TPS ranges from 125.94 to 2042.33 (high variance)
- **load-2.db**: TPS ranges from 137.11 to 1639.16 (more stable)
- **Consistency**: load-2 shows more consistent performance

### Cost Analysis
- All transactions use standard gas: 21,000 units (ETH transfer)
- Gas price is very low (< 1 Gwei) in load-1 and load-2
- transactions.db shows 2 Gwei gas price but no successful txns

---

## 📁 Files Reference

```
/home/tosif/Desktop/oss/go-tps/
├── analyze_multi_db.py          # Main analysis script
├── db_summary.csv               # Generated CSV report (YOUR OUTPUT)
├── DB_ANALYSIS_README.md        # Detailed column documentation
├── usage_examples.sh            # Usage examples and quick reference
├── load-1.db                    # Database 1 (179,900 txns)
├── load-2.db                    # Database 2 (539,700 txns)
└── transactions.db              # Database 3 (5,717 txns)
```

---

## 🔧 Customization

### Add More Databases
Simply add more .db files to the command:
```bash
python3 analyze_multi_db.py db1.db db2.db db3.db db4.db db5.db
```

### Modify Metrics
Edit `analyze_multi_db.py` to add custom calculations in the `analyze_database()` function.

### Change CSV Format
Modify the `columns` list in `generate_csv()` function to reorder or remove columns.

---

## 📖 Documentation

- **DB_ANALYSIS_README.md** - Detailed explanation of all metrics
- **usage_examples.sh** - Quick command reference
- **claude.md** - Full project documentation (existing file)

---

## ⚠️ Important Notes

1. **Gas Price Units**: 
   - Wei: Smallest unit (1 ETH = 10^18 wei)
   - Gwei: Common unit (1 Gwei = 10^9 wei)

2. **Confirmation Latency**: 
   - Time from submission to blockchain confirmation
   - Lower is better for user experience

3. **TPS Metrics**:
   - Submission TPS: How fast transactions were sent
   - Confirmation TPS: How fast blockchain confirmed them
   - Batch TPS: Per-batch variation (min/max/avg)

4. **Success Rate**:
   - 100% = All transactions confirmed
   - <100% = Some failed or still pending
   - 0% = None confirmed (check network/config)

---

## 🎯 Next Steps

1. **Analyze CSV**: Open `db_summary.csv` in your preferred tool
2. **Compare Metrics**: Look at TPS, latency, and success rates
3. **Identify Issues**: Low success rates or high latency indicate problems
4. **Optimize**: Use insights to tune your configuration
5. **Re-run**: Test with new settings and compare results

---

## 🆘 Troubleshooting

### Script Errors
```bash
# Check Python version (requires 3.6+)
python3 --version

# Verify database files exist
ls -lh *.db

# Test with single database first
python3 analyze_multi_db.py load-1.db
```

### CSV Issues
```bash
# Check CSV format
head db_summary.csv

# Verify number of columns
head -1 db_summary.csv | tr ',' '\n' | wc -l

# Check for errors in CSV
python3 -c "import csv; csv.DictReader(open('db_summary.csv'))"
```

### Missing Data
- If columns show 0 or empty: Database may lack that metric
- If success_rate is 0%: Transactions not confirmed
- If TPS is 0: Check submission/confirmation windows

---

## 📞 Support

For issues or questions:
1. Check **DB_ANALYSIS_README.md** for column explanations
2. Run `./usage_examples.sh` for command reference
3. Review **claude.md** for full project documentation
4. Check database structure with: `sqlite3 database.db ".schema"`

---

**Created:** $(date)
**Script:** analyze_multi_db.py
**Output:** db_summary.csv (48 columns)
**Databases Analyzed:** 3 (load-1.db, load-2.db, transactions.db)
