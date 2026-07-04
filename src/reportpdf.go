package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/smart"
)

/*
	reportpdf.go

	Server-side PDF rendering of the system analytic report. Reuses the same
	AnalyticReport structure as the JSON API and is served from
	GET /api/info/report?format=pdf as a file download.

	The PDF uses the built-in Helvetica font, so non latin-1 characters are
	replaced before writing. All report content (device names, paths, SMART
	fields) is ASCII in practice.
*/

const (
	pdfMarginLeft  = 14.0
	pdfMarginTop   = 16.0
	pdfMarginRight = 14.0
	pdfPageWidth   = 210.0 // A4 portrait
	pdfContentW    = pdfPageWidth - pdfMarginLeft - pdfMarginRight
)

// pdfSanitize makes a string safe for the latin-1 core fonts.
func pdfSanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r > 255 {
			r = '?'
		}
		out = append(out, r)
	}
	return string(out)
}

func pdfValue(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return pdfSanitize(s)
}

func pdfBytesToHuman(size int64) string {
	if size < 0 {
		return "-"
	}
	units := []string{"B", "kB", "MB", "GB", "TB", "PB"}
	f := float64(size)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

type reportPdfWriter struct {
	pdf *fpdf.Fpdf
}

func (rw *reportPdfWriter) sectionHeader(title string) {
	rw.pdf.SetFont("Helvetica", "B", 13)
	rw.pdf.SetTextColor(30, 30, 30)
	rw.pdf.CellFormat(pdfContentW, 9, pdfSanitize(title), "", 1, "L", false, 0, "")
	rw.pdf.SetLineWidth(0.4)
	rw.pdf.SetDrawColor(80, 80, 80)
	y := rw.pdf.GetY()
	rw.pdf.Line(pdfMarginLeft, y, pdfMarginLeft+pdfContentW, y)
	rw.pdf.Ln(2)
}

func (rw *reportPdfWriter) subHeader(title string) {
	rw.pdf.SetFont("Helvetica", "B", 11)
	rw.pdf.SetTextColor(50, 50, 50)
	rw.pdf.CellFormat(pdfContentW, 8, pdfSanitize(title), "", 1, "L", false, 0, "")
}

// kvRow writes a two-column key / value table row.
func (rw *reportPdfWriter) kvRow(key, value string) {
	keyW := pdfContentW * 0.32
	rw.pdf.SetFont("Helvetica", "B", 9)
	rw.pdf.SetFillColor(244, 244, 244)
	rw.pdf.CellFormat(keyW, 6.5, pdfSanitize(key), "1", 0, "L", true, 0, "")
	rw.pdf.SetFont("Helvetica", "", 9)
	rw.pdf.MultiCell(pdfContentW-keyW, 6.5, pdfValue(value), "1", "L", false)
}

// tableHeader writes a table heading row with the given column widths.
func (rw *reportPdfWriter) tableHeader(headers []string, widths []float64) {
	rw.pdf.SetFont("Helvetica", "B", 9)
	rw.pdf.SetFillColor(230, 230, 230)
	for i, h := range headers {
		rw.pdf.CellFormat(widths[i], 6.5, pdfSanitize(h), "1", 0, "L", true, 0, "")
	}
	rw.pdf.Ln(-1)
}

func (rw *reportPdfWriter) tableRow(cells []string, widths []float64) {
	rw.pdf.SetFont("Helvetica", "", 9)
	for i, c := range cells {
		rw.pdf.CellFormat(widths[i], 6.5, pdfValue(c), "1", 0, "L", false, 0, "")
	}
	rw.pdf.Ln(-1)
}

func formatUptimePdf(sec uint64) string {
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	return fmt.Sprintf("%dd %dh %dm", d, h, m)
}

// GenerateReportPDF renders the analytic report into a PDF document.
func GenerateReportPDF(report *AnalyticReport) (*fpdf.Fpdf, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfMarginLeft, pdfMarginTop, pdfMarginRight)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 8, fmt.Sprintf("BokoDM System Analytics Report - Page %d", pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	rw := &reportPdfWriter{pdf: pdf}

	// Title block
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(pdfContentW, 10, "System Analytics Report", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(100, 100, 100)
	hostname := ""
	if report.Host != nil {
		hostname = report.Host.Hostname
	}
	pdf.CellFormat(pdfContentW, 6, pdfSanitize(fmt.Sprintf("Generated at %s on %s", report.GeneratedAt.Format("2006-01-02 15:04:05"), hostname)), "", 1, "L", false, 0, "")
	pdf.CellFormat(pdfContentW, 6, pdfSanitize("System UUID: "+report.SystemUUID), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// 1. System information
	rw.sectionHeader("1. System Information")
	if host := report.Host; host != nil {
		rw.kvRow("Hostname", host.Hostname)
		rw.kvRow("Operating System", host.Platform+" "+host.PlatformVersion)
		rw.kvRow("Kernel", host.KernelVersion+" ("+host.KernelArch+")")
		rw.kvRow("CPU", fmt.Sprintf("%s x %d threads", host.CPUModel, host.CPUCores))
		rw.kvRow("Total Memory", pdfBytesToHuman(int64(host.TotalMemory)))
		rw.kvRow("Uptime", formatUptimePdf(host.UptimeSec))
		if host.BootTime > 0 {
			rw.kvRow("Boot Time", time.Unix(int64(host.BootTime), 0).Format("2006-01-02 15:04:05"))
		}
	}
	pdf.Ln(5)

	// 2. Network interfaces
	rw.sectionHeader("2. Network Interfaces")
	ifaceWidths := []float64{pdfContentW * 0.1, pdfContentW * 0.25, pdfContentW * 0.65}
	rw.tableHeader([]string{"ID", "Interface", "IP Addresses"}, ifaceWidths)
	for _, iface := range report.NetworkInterfaces {
		rw.tableRow([]string{fmt.Sprintf("%d", iface.ID), iface.Name, strings.Join(iface.IPs, ", ")}, ifaceWidths)
	}
	pdf.Ln(5)

	// 3. RAID arrays
	rw.sectionHeader("3. RAID Arrays")
	if len(report.RAIDArrays) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(pdfContentW, 6, "No RAID array configured on this system.", "", 1, "L", false, 0, "")
	}
	for _, raidInfo := range report.RAIDArrays {
		rw.subHeader(fmt.Sprintf("%s (%s)", raidInfo.DevicePath, strings.ToUpper(raidInfo.RaidLevel)))
		rw.kvRow("Array Name", raidInfo.Name)
		rw.kvRow("UUID", raidInfo.UUID)
		rw.kvRow("State", raidInfo.State)
		rw.kvRow("Array Size", pdfBytesToHuman(int64(raidInfo.ArraySize)*1024))
		rw.kvRow("Devices", fmt.Sprintf("%d active / %d working / %d failed / %d spare",
			raidInfo.ActiveDevices, raidInfo.WorkingDevices, raidInfo.FailedDevices, raidInfo.SpareDevices))
		memberWidths := []float64{pdfContentW * 0.4, pdfContentW * 0.2, pdfContentW * 0.4}
		rw.tableHeader([]string{"Member Device", "Raid Device No.", "State"}, memberWidths)
		for _, member := range raidInfo.DeviceInfo {
			rw.tableRow([]string{member.DevicePath, fmt.Sprintf("%d", member.RaidDevice), strings.Join(member.State, ", ")}, memberWidths)
		}
		pdf.Ln(3)
	}
	pdf.Ln(2)

	// 4. Disks & partitions
	rw.sectionHeader("4. Disks & Partitions")
	for idx, diskReport := range report.Disks {
		disk := diskReport.Disk
		rw.subHeader(fmt.Sprintf("4.%d  /dev/%s - %s", idx+1, disk.Name, disk.Model))
		rw.kvRow("Identifier", disk.Identifier)
		rw.kvRow("Size", pdfBytesToHuman(disk.Size))
		rw.kvRow("Used", pdfBytesToHuman(disk.Used))
		rw.kvRow("Disk Label", disk.DiskLabel)

		if health := diskReport.SmartHealth; health != nil {
			overall := "FAILING"
			if health.IsHealthy {
				overall = "PASSED"
			}
			mediaType := "HDD"
			if health.IsNVMe {
				mediaType = "NVMe"
			} else if health.IsSSD {
				mediaType = "SSD"
			}
			rw.kvRow("SMART Overall Health", overall)
			rw.kvRow("Serial Number", health.SerialNumber)
			rw.kvRow("Power-on Hours", fmt.Sprintf("%d hours", health.PowerOnHours))
			rw.kvRow("Power Cycles", fmt.Sprintf("%d", health.PowerCycleCount))
			rw.kvRow("Reallocated Sectors", fmt.Sprintf("%d", health.ReallocatedSectors))
			rw.kvRow("Pending Sectors", fmt.Sprintf("%d", health.PendingSectors))
			rw.kvRow("Uncorrectable Errors", fmt.Sprintf("%d", health.UncorrectableErrors))
			rw.kvRow("UDMA CRC Errors", fmt.Sprintf("%d", health.UDMACRCErrors))
			rw.kvRow("Media Type", mediaType)
		}

		// SMART identity block, type depends on the disk bus
		if sataInfo, ok := diskReport.SmartInfo.(*smart.SATADiskInfo); ok && sataInfo != nil {
			rw.kvRow("Model Family", sataInfo.ModelFamily)
			rw.kvRow("Device Model", sataInfo.DeviceModel)
			rw.kvRow("Firmware", sataInfo.Firmware)
			rw.kvRow("User Capacity", sataInfo.UserCapacity)
			rw.kvRow("Rotation Rate", sataInfo.RotationRate)
			rw.kvRow("Form Factor", sataInfo.FormFactor)
		} else if nvmeInfo, ok := diskReport.SmartInfo.(*smart.NVMEInfo); ok && nvmeInfo != nil {
			rw.kvRow("Model Number", nvmeInfo.ModelNumber)
			rw.kvRow("Firmware Version", nvmeInfo.FirmwareVersion)
			rw.kvRow("NVMe Version", nvmeInfo.NVMeVersion)
			rw.kvRow("Total Capacity", nvmeInfo.TotalNVMeCapacity)
		}

		if len(disk.Partitions) > 0 {
			partWidths := []float64{pdfContentW * 0.13, pdfContentW * 0.11, pdfContentW * 0.12, pdfContentW * 0.12, pdfContentW * 0.22, pdfContentW * 0.3}
			rw.tableHeader([]string{"Partition", "FS", "Size", "Used", "Mount Point", "UUID"}, partWidths)
			for _, part := range disk.Partitions {
				rw.tableRow([]string{part.Name, part.FsType, pdfBytesToHuman(part.Size), pdfBytesToHuman(part.Used), part.MountPoint, part.UUID}, partWidths)
			}
		} else {
			pdf.SetFont("Helvetica", "I", 9)
			pdf.SetTextColor(100, 100, 100)
			pdf.CellFormat(pdfContentW, 6, "No partition on this disk.", "", 1, "L", false, 0, "")
		}
		pdf.Ln(3)
	}
	pdf.Ln(2)

	// 5. System tools
	rw.sectionHeader("5. System Tools")
	if report.Dependencies != nil {
		depWidths := []float64{pdfContentW * 0.25, pdfContentW * 0.15, pdfContentW * 0.6}
		rw.tableHeader([]string{"Tool", "Status", "Purpose"}, depWidths)
		for _, dep := range report.Dependencies.Deps {
			state := "Missing"
			if dep.Found {
				state = "Installed"
			}
			rw.tableRow([]string{dep.Name, state, dep.Description}, depWidths)
		}
	}

	if pdf.Err() {
		return nil, pdf.Error()
	}
	return pdf, nil
}

// servePDFReport writes the report as a downloadable PDF file.
func servePDFReport(w http.ResponseWriter, report *AnalyticReport) {
	pdf, err := GenerateReportPDF(report)
	if err != nil {
		http.Error(w, "PDF generation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := "bokodm_report_" + report.GeneratedAt.Format("20060102_150405") + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	if err := pdf.Output(w); err != nil {
		http.Error(w, "PDF output failed: "+err.Error(), http.StatusInternalServerError)
	}
}
