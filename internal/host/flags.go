package host

import "fmt"

// Aikar's G1 flags, the usual tuning for Minecraft servers: a fixed heap
// (-Xms = -Xmx, touched up front) and young-generation sizes that suit its
// short-lived garbage. Heaps over 12 GB get larger regions and a larger
// young generation so G1 doesn't over-collect.
// https://docs.papermc.io/paper/aikars-flags

// bigHeapMB is where the large-heap values start.
const bigHeapMB = 12 << 10

// aikarFlags are the JVM flags for a server with memMB of heap.
func aikarFlags(memMB int) []string {
	newSize, maxNew, region, reserve, occupancy := 30, 40, "8M", 20, 15
	if memMB > bigHeapMB {
		newSize, maxNew, region, reserve, occupancy = 40, 50, "16M", 15, 20
	}
	return []string{
		fmt.Sprintf("-Xms%dM", memMB), fmt.Sprintf("-Xmx%dM", memMB),
		"-XX:+UseG1GC", "-XX:+ParallelRefProcEnabled", "-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions", "-XX:+DisableExplicitGC", "-XX:+AlwaysPreTouch",
		fmt.Sprintf("-XX:G1NewSizePercent=%d", newSize), fmt.Sprintf("-XX:G1MaxNewSizePercent=%d", maxNew),
		"-XX:G1HeapRegionSize=" + region, fmt.Sprintf("-XX:G1ReservePercent=%d", reserve),
		"-XX:G1HeapWastePercent=5", "-XX:G1MixedGCCountTarget=4",
		fmt.Sprintf("-XX:InitiatingHeapOccupancyPercent=%d", occupancy),
		"-XX:G1MixedGCLiveThresholdPercent=90", "-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32", "-XX:+PerfDisableSharedMem", "-XX:MaxTenuringThreshold=1",
	}
}
