// Package implements browsing ext4 filesystem from disk or database
// Author : Prashant Shah <pshah.mumbai@gmail.com>
package main

// Notes :
// Inode number is always 32 bit
// Block number can be 64 bit
// Total Blocks can be 64 bit
// Total Inodes is always 32 bit
// File Size can be 64 bit
// File Block count can be 64 bit
// Inode - Direct/Indirect addresses are 32 bit (cannot be placed higher than 2^32 blocks)

// To Do :
// Check bg_flags of ext4_group_desc
// Cluster support - only bitmaps in BG track clusters not blocks
// Huge file - inode
// Compression - inode

import (
	_ "../../server/src/db"
	"database/sql"
	"fmt"
	"os"
	//_ "github.com/ziutek/mymysql/godrv"
)

const ( // Database connection details
	DB_NAME = "testnucdp"
	DB_USER = "root"
	DB_PASS = "root"
)

const (
	RootInode = 2 // Inode number of root '/' directory
)

const ( // Type of inode - file, directory, device, etc
	FileT = 1
	DirT  = 2
)

// File system meta data
type FileSystem struct {
	TotalInode          uint64 // Total number of inodes
	TotalBlocks         uint64 // Total number of blocks - 64 bit
	BlockSize           uint64 // Block size - 32 bit
	ClusterSize         uint64 // Cluster size - 32 bit
	BlocksPerBlkGroup   uint64 // Blocks per block group
	InodesPerBlkGroup   uint64 // Inodes per block group
	InodeSize           uint64 // Inode size
	TotalBlockGroups    uint64 // Total number of block groups
	StartBGDT           uint64 // Start of Block Group Descriptor Table
	BlockGroupDescSize  uint64 // Size of each block group descriptor
	BlockDescPerBlock   uint64 // Block descriptors per block
	FeaturesPerformance uint32 // Flags that can be ingnored
	FeaturesReadWrite   uint32 // Flags required to read and write
	FeaturesRead        uint32 // Flags required to write, if not present can mount read-only

	// Flags - Features need to read and write
	Compression        bool // FeaturesReadWrite
	TypeInDirEntry     bool // FeaturesReadWrite
	MetaBlockGroups    bool // FeaturesReadWrite
	Extents            bool // FeaturesReadWrite
	FSSize64           bool // FeaturesReadWrite
	FlexibleBlockGroup bool // FeaturesReadWrite
	DirEntryData       bool // FeaturesReadWrite
	LargeDirectory     bool // FeaturesReadWrite
	DataInInode        bool // FeaturesReadWrite

	// Flags - Features needed to read
	SparseSuper      bool // FeaturesRead
	Use64BitSize     bool // FeaturesRead
	FileSizeInBlocks bool // FeaturesRead
	SubDirLimit      bool // FeaturesRead
	LargeInode       bool // FeaturesRead
	BigAlloc         bool // FeaturesRead

	FirstMetaBG uint64
	FlexBGSize  uint64

	Signature    uint32 // File system signature
	MajorVersion uint32 // Major version
	MinorVersion uint32 // Minor version
}

var fs FileSystem

// Block read from disk or database
type Nucdp struct {
	Id           int
	SnapshotID   int
	GlobalSnapID int
	Dtime        string
	BlockNum     int
	SectorNum    int
	BlockSize    int
	OpType       string
	Data         []byte // The data returned from disk or database is stored here
}

// Block Group descriptor
type BlockDesc struct {
	BlockBitmap uint64 // Starting block for the block bitmap - 1 block size - 64 bit
	InodeBitmap uint64 // Starting block for the inode bitmap - 1 block size - 64 bit
	InodeTable  uint64 // Starting block for the inode table - 64 bit
}

// Block Group Descriptor Map
var BlockDescMap []BlockDesc

// Inode
type Inode struct {
	Size       uint64 // Size of inode itself
	Data       []byte // Full inode content
	DataSize   uint64 // Size of data item inode represents
	BlockNum   uint64 // Block number of inode
	BlockCount uint64 // TO DO
	Flags      uint32
	Flag       struct {
		Compressed        bool
		CompressedCluster bool
		DirHashIndexes    bool
		HugeFile          bool
		UsesExtents       bool
	}
	Direct         [12]uint64 // Direct mapping
	SingleIndirect uint64     // Single indirect mapping
	DoubleIndirect uint64     // Double indirect mapping
	TripeIndirect  uint64     // Tripple indirect mapping
}

// Extent Header at start of every extent block
type ExtentHeader struct {
	Magic      uint32 // Magin number
	Entries    uint32 // Number of valid entries that follows
	Max        uint32 // Max number of entries that it can hold
	Depth      uint32 // Depth of the header 0 = leaf nodes
	Generation uint32
}

// Leaf extent that point to data block
type Extent struct {
	FileBlock uint32 // 32 bit - File Offset
	Len       uint32
	Block     uint64 // 64 bit - Block Number
}

// Interior extents that point to extent blocks
type ExtentIdx struct {
	FileBlock uint32 // 32 bit - File Offset
	Block     uint64 // 64 bit - Block Number
}

// Single Directory entry
type DirectoryEntry struct {
	Id          uint32 // Id number used in selection
	InodeNumber uint64 // Inode number of the entry
	EntrySize   uint32 // Size of the entire entry
	NameLength  uint32 // Size of the name field that follows
	Type        uint32 // Type of the entry
	Name        []byte // Name of the entry
}

// List of all the dictory entries of a directory inode
var DirList [33000]DirectoryEntry

// Index into the Dir List
var DirIndex uint32

var db *sql.DB

var devFile *os.File
var dumpFile *os.File

var DataSrc string

func main() {
	var err error

	DataSrc = "file" // "db" or "file"

	if DataSrc == "file" {
		// Open block device
		devFile, err = os.Open("/dev/loop0")
		if err != nil {
			fmt.Println("Error opening partition", err)
			return
		}
		defer devFile.Close()
	} else {
		// Open database connection
		//db, err = sql.Open("mymysql", fmt.Sprintf("%s/%s/%s", DB_NAME, DB_USER, DB_PASS))
		db, err = sql.Open("sqlite3", "/home/user/Public/1.db")
		if err != nil {
			fmt.Println("Error connecting to database", err)
			return
		}
		defer db.Close()
	}

	// Read superblock
	if !ReadSuper() {
		return
	}

	// Parse BGDT
	if !ParseBGDT() {
		return
	}

	// List root directory
	if !ReadInode(RootInode, DirT) {
		fmt.Println("Error reading root directory")
		return
	}

	var input int   // Read user input
	var isFile bool // Whether input selected is a file or not
	for {
		fmt.Printf("Enter your choice (-1 to exit): ")
		fmt.Scanf("%d", &input)
		if input == -1 {
			break
		}
		if !(DirList[input].Type == FileT || DirList[input].Type == DirT) {
			fmt.Printf("File type not supported %d\n", DirList[input].Type)
			continue
		}
		if DirList[input].Type == FileT {
			isFile = true
		} else {
			isFile = false
		}
		if isFile { // Open file for dumping if the item is file
			dumpFile, err = os.Create(string(DirList[input].Name))
			if err != nil {
				fmt.Println("Error opening file for writing : %s", DirList[input].Name)
				continue
			}
		}
		// Read the inode corresponding to the item selected by user
		if !ReadInode(DirList[input].InodeNumber, DirList[input].Type) {
			fmt.Println("Error reading inode")
			return
		}
		if isFile { // Close file after dumping the data
			fmt.Println("Done writing")
			dumpFile.Close()
		}
	}

}

// Read superblock
func ReadSuper() bool {
	var n Nucdp

	if DataSrc == "file" {
		n.Data = make([]byte, 4096)
	}

	if !ReadBlock(0, &n) {
		fmt.Println("Error reading super block")
		return false
	}

	// Parse file system information
	b := 1024 // Base offset is 1024
	fs.TotalInode = uint64(ToInt(n.Data[b : b+4]))
	fs.BlockSize = uint64(ToInt(n.Data[b+24 : b+28]))
	fs.BlockSize = 1024 << fs.BlockSize // Converting from log to decimal
	fs.BlocksPerBlkGroup = uint64(ToInt(n.Data[b+32 : b+36]))
	fs.InodesPerBlkGroup = uint64(ToInt(n.Data[b+40 : b+44]))
	fs.FeaturesPerformance = ToInt(n.Data[b+92 : b+96])
	fs.FeaturesReadWrite = ToInt(n.Data[b+96 : b+100])
	fs.FeaturesRead = ToInt(n.Data[b+100 : b+104])
	fs.Signature = ToInt(n.Data[b+56 : b+58])
	fs.MajorVersion = ToInt(n.Data[b+76 : b+80])
	fs.MinorVersion = ToInt(n.Data[b+62 : b+64])

	//////////////// FILE SYSTEM FEATURES //////////////

	fs.Compression = false
	fs.TypeInDirEntry = false
	fs.MetaBlockGroups = false
	fs.Extents = false
	fs.FSSize64 = false
	fs.FlexibleBlockGroup = false
	fs.DirEntryData = false
	fs.LargeDirectory = false
	fs.DataInInode = false

	if (fs.FeaturesReadWrite & (1 << 0)) != 0 { // 1 bit
		fs.Compression = true
	}
	if (fs.FeaturesReadWrite & (1 << 1)) != 0 { // 2 bit
		fs.TypeInDirEntry = true
	}
	if (fs.FeaturesReadWrite & (1 << 4)) != 0 { // 5 bit
		fs.MetaBlockGroups = true
	}
	if (fs.FeaturesReadWrite & (1 << 6)) != 0 { // 7 bit
		fs.Extents = true
	}
	if (fs.FeaturesReadWrite & (1 << 7)) != 0 { // 8 bit
		fs.FSSize64 = true
	}
	if (fs.FeaturesReadWrite & (1 << 9)) != 0 { // 10 bit
		fs.FlexibleBlockGroup = true
	}
	if (fs.FeaturesReadWrite & (1 << 12)) != 0 { // 13 bit
		fs.DirEntryData = true
	}
	if (fs.FeaturesReadWrite & (1 << 14)) != 0 { // 15 bit
		fs.LargeDirectory = true
	}
	if (fs.FeaturesReadWrite & (1 << 15)) != 0 { // 16 bit
		fs.DataInInode = true
	}

	fs.SparseSuper = false
	fs.Use64BitSize = false
	fs.FileSizeInBlocks = false
	fs.SubDirLimit = false
	fs.LargeInode = false
	fs.BigAlloc = false

	if (fs.FeaturesRead & (1 << 0)) != 0 { // 1 bit
		fs.SparseSuper = true
	}
	if (fs.FeaturesRead & (1 << 1)) != 0 { // 2 bit
		fs.Use64BitSize = true
	}
	if (fs.FeaturesRead & (1 << 3)) != 0 { // 4 bit
		fs.FileSizeInBlocks = true
	}
	if (fs.FeaturesRead & (1 << 5)) != 0 { // 6 bit
		fs.SubDirLimit = true
	}
	if (fs.FeaturesRead & (1 << 6)) != 0 { // 7 bit
		fs.LargeInode = true
	}
	if (fs.FeaturesRead & (1 << 9)) != 0 { // 10 bit
		fs.BigAlloc = true
	}

	// Check if its a valid ext partition
	if fs.Signature != 0xEF53 {
		fmt.Println("Not a valid ext2/ext3/ext4 file system")
		return false
	}

	// If version < 1.0 then inode size fixed as 128
	if fs.MajorVersion < 1 {
		fs.InodeSize = 128
	} else {
		fs.InodeSize = uint64(ToInt(n.Data[b+88 : b+92]))
	}

	// Calculate total block count
	if fs.FSSize64 {
		var highValue, lowValue uint64
		lowValue = uint64(ToInt(n.Data[b+4 : b+8]))
		highValue = uint64(ToInt(n.Data[b+336 : b+340]))
		fs.TotalBlocks = (highValue << 32) | lowValue
	} else {
		fs.TotalBlocks = uint64(ToInt(n.Data[b+4 : b+8]))
	}

	// Calculate total block groups
	fs.TotalBlockGroups = fs.TotalBlocks / fs.BlocksPerBlkGroup
	if (fs.TotalBlocks % fs.BlocksPerBlkGroup) != 0 {
		fs.TotalBlockGroups++
	}

	// Calculate the starting block of the Block Group Descriptor Table
	if fs.BlockSize <= 1024 {
		fs.StartBGDT = 2
	} else {
		fs.StartBGDT = 1
	}

	// Calculate block group descriptor size
	fs.BlockGroupDescSize = 32 // Default size
	if fs.FSSize64 {
		fs.BlockGroupDescSize = uint64(ToInt(n.Data[b+254 : b+256]))
		if fs.BlockGroupDescSize < 32 {
			fs.BlockGroupDescSize = 32
		}
	}

	// Calculate number of block descriptors per block
	fs.BlockDescPerBlock = fs.BlockSize / fs.BlockGroupDescSize

	if fs.BigAlloc {
		fs.ClusterSize = 1024 << ToInt(n.Data[b+28:b+32])
	}

	if fs.MetaBlockGroups {
		fs.FirstMetaBG = uint64(ToInt(n.Data[b+260 : b+264]))
	}

	if fs.FlexibleBlockGroup {
		fs.FlexBGSize = 1 << ToInt(n.Data[b+372:b+373])
	}

	fmt.Println("Total Inode  : ", fs.TotalInode)
	fmt.Println("Total Blocks : ", fs.TotalBlocks)
	fmt.Println("Block Size   : ", fs.BlockSize)
	fmt.Println("Cluster Size : ", fs.ClusterSize)
	fmt.Println("BlkPerBlkGrp : ", fs.BlocksPerBlkGroup)
	fmt.Println("InodePerBlkG : ", fs.InodesPerBlkGroup)
	fmt.Println("InodeSize    : ", fs.InodeSize)
	fmt.Println("TotalBlkGrp  : ", fs.TotalBlockGroups)
	fmt.Println("BlkGDesSize  : ", fs.BlockGroupDescSize)
	fmt.Println("BlkDesPerBlk : ", fs.BlockDescPerBlock)
	fmt.Println("FeatPerf     : ", fs.FeaturesPerformance)
	fmt.Println("FeatRdWr     : ", fs.FeaturesReadWrite)
	fmt.Println("FeatWrite    : ", fs.FeaturesRead)
	fmt.Println("Signature    : ", fs.Signature)
	fmt.Println("MajorVersion : ", fs.MajorVersion)
	fmt.Println("MinorVersion : ", fs.MinorVersion)

	fmt.Println("Compression        : ", fs.Compression)
	fmt.Println("TypeInDirEntry     : ", fs.TypeInDirEntry)
	fmt.Println("MetaBlockGroups    : ", fs.MetaBlockGroups)
	fmt.Println("Extents            : ", fs.Extents)
	fmt.Println("FSSize64           : ", fs.FSSize64)
	fmt.Println("FlexibleBlockGroup : ", fs.FlexibleBlockGroup)
	fmt.Println("DirEntryData       : ", fs.DirEntryData)
	fmt.Println("LargeDirectory     : ", fs.LargeDirectory)
	fmt.Println("DataInInode        : ", fs.DataInInode)

	fmt.Println("SparseSuper        : ", fs.SparseSuper)
	fmt.Println("Use64BitSize       : ", fs.Use64BitSize)
	fmt.Println("FileSizeInBlocks   : ", fs.FileSizeInBlocks)
	fmt.Println("SubDirLimit        : ", fs.SubDirLimit)
	fmt.Println("LargeInode         : ", fs.LargeInode)
	fmt.Println("BigAlloc           : ", fs.BigAlloc)

	fmt.Println("FirstMetaBG        : ", fs.FirstMetaBG)
	fmt.Println("FlexBGSize         : ", fs.FlexBGSize)

	return true
}

// Parse Block Group Descriptor Table
func ParseBGDT() bool {
	var n Nucdp
	var offset uint64
	var currentBlock uint64       // Block number containing the BGT entries
	var metaBlockGrCounter uint64 // Current meta block group if meta block group enabled
	var highValue, lowValue uint64

	currentBlock = fs.StartBGDT
	metaBlockGrCounter = 0

	BlockDescMap = make([]BlockDesc, fs.TotalBlockGroups)

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
	}

	// Process BGDT Entries
	count := uint64(0)
	for i := uint64(0); i < fs.TotalBlockGroups; i++ {

		// Read block containing BGDT entries when count = 0
		if count == 0 {
			if fs.MetaBlockGroups {
				// If meta block groups is enabled then BGDT
				// is split across 1st block of each meta block
				// groups and is size is max one block
				currentBlock = metaBlockGrCounter * (fs.BlockSize / fs.BlockGroupDescSize) * fs.BlocksPerBlkGroup

				// If sparsesuper is enabled then backup copies
				// of super block are at 0, 1 and power of 3,5,7
				// else backup super block at each block group.
				// If superblock is present then BGDT is offset
				// by 1 block
				if fs.SparseSuper {
					if BlockContainsSuper(currentBlock) {
						currentBlock++
					}
				} else {
					currentBlock++
				}
			}

			fmt.Println("BGDT Block Number =", currentBlock)
			if !ReadBlock(currentBlock, &n) { // Read next file system block
				fmt.Println("Error reading block group descriptor")
				return false
			}

			if fs.MetaBlockGroups {
				// Increament meta block group counter
				metaBlockGrCounter++
			} else {
				// If meta block groups is disabled then BGDT
				// blocks are sequential from the 1st block for
				// entire file system
				currentBlock++
			}
		}

		offset = count * fs.BlockGroupDescSize

		if (fs.BlockGroupDescSize > 32) && (fs.FSSize64 == true) {
			lowValue = uint64(ToInt(n.Data[offset : offset+4]))
			highValue = uint64(ToInt(n.Data[offset+32 : offset+36]))
			BlockDescMap[i].BlockBitmap = (highValue << 32) | lowValue
		} else {
			BlockDescMap[i].BlockBitmap = uint64(ToInt(n.Data[offset : offset+4]))
		}

		if (fs.BlockGroupDescSize > 32) && (fs.FSSize64 == true) {
			lowValue = uint64(ToInt(n.Data[offset+4 : offset+8]))
			highValue = uint64(ToInt(n.Data[offset+36 : offset+40]))
			BlockDescMap[i].InodeBitmap = (highValue << 32) | lowValue
		} else {
			BlockDescMap[i].InodeBitmap = uint64(ToInt(n.Data[offset+4 : offset+8]))
		}

		if (fs.BlockGroupDescSize > 32) && (fs.FSSize64 == true) {
			lowValue = uint64(ToInt(n.Data[offset+8 : offset+12]))
			highValue = uint64(ToInt(n.Data[offset+40 : offset+44]))
			BlockDescMap[i].InodeTable = (highValue << 32) | lowValue
		} else {
			BlockDescMap[i].InodeTable = uint64(ToInt(n.Data[offset+8 : offset+12]))
		}

		count++

		// Check if we are at end of the current block
		if count >= fs.BlockDescPerBlock {
			count = 0
		}
	}

	fmt.Println("Block Group Descriptor Map :")
	for c, v := range BlockDescMap {
		fmt.Println(c, v)
	}
	return true
}

// Read Inode
func ReadInode(inodeNum uint64, inodeType uint32) bool {
	var n Nucdp
	var i Inode

	fmt.Println("ReadInode : InodeNumber =", inodeNum, " Type =", inodeType)

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
	}

	// If Directory then reset the DirList and print the DirList at end of function
	if inodeType == DirT {
		// Reset DirList
		for c, _ := range DirList {
			DirList[c].Id = 0
			DirList[c].InodeNumber = 0
			DirList[c].Name = nil
			DirList[c].Type = 0
		}
		DirIndex = 0

		defer PrintDirList()
	}

	// Calculating inode table index
	curBlockGroup := (inodeNum - 1) / fs.InodesPerBlkGroup // Inode number starts from 1
	indexInodeTable := (inodeNum - 1) % fs.InodesPerBlkGroup

	// Calculating exact block containing the inode
	blockStartInodeTable := BlockDescMap[curBlockGroup].InodeTable
	blockOffset := (indexInodeTable * fs.InodeSize) / fs.BlockSize
	blockNum := blockStartInodeTable + blockOffset

	if !ReadBlock(blockNum, &n) {
		fmt.Println("Error reading Inode")
		return false
	}

	// Inode offset
	inodeBlockOffset := (indexInodeTable * fs.InodeSize) % fs.BlockSize

	// fmt.Println(inodeNum, i.blockNum, curBlockGroup, indexInodeTable, blockStartInodeTable, blockOffset, inodeBlockOffset)
	// Read inode
	i.Data = n.Data[inodeBlockOffset : inodeBlockOffset+fs.InodeSize]

	// Calculating data size
	var lowValue, highValue uint64
	if fs.Use64BitSize { // If using 64 bit size
		lowValue = uint64(ToInt(i.Data[4:8]))
		highValue = uint64(ToInt(i.Data[108:112]))
		if highValue > 0 {
			i.DataSize = (highValue << 32) | lowValue
		} else {
			i.DataSize = lowValue
		}
	} else {
		i.DataSize = uint64(ToInt(i.Data[4:8]))
	}
	fmt.Println("Datasize =", i.DataSize)

	// Calculate block count
	lowValue = uint64(ToInt(i.Data[28:32]))
	highValue = uint64(ToInt(i.Data[116:118]))
	if highValue > 0 {
		i.BlockCount = (highValue << 32) | lowValue
	} else {
		i.BlockCount = lowValue
	}
	fmt.Println("BlockCount =", i.BlockCount)

	// Inode size as stored in inode
	i.Size = uint64(ToInt(i.Data[128:130]))
	i.Size += 128
	fmt.Println("InodeSize =", i.Size)

	// Calculate features
	i.Flags = ToInt(i.Data[32:36])
	i.Flag.Compressed = false
	i.Flag.CompressedCluster = false
	i.Flag.DirHashIndexes = false
	i.Flag.HugeFile = false
	i.Flag.UsesExtents = false
	if (i.Flags & (1 << 2)) != 0 { // 3 bit
		i.Flag.Compressed = true
	}
	if (i.Flags & (1 << 9)) != 0 { // 10 bit
		i.Flag.CompressedCluster = true
	}
	if (i.Flags & (1 << 12)) != 0 { // 13 bit
		i.Flag.DirHashIndexes = true
	}
	if (i.Flags & (1 << 18)) != 0 { // 19 bit
		i.Flag.HugeFile = true
	}
	if (i.Flags & (1 << 19)) != 0 { // 20 bit
		i.Flag.UsesExtents = true
	}
	fmt.Println("Inode.Compression       =", i.Flag.Compressed)
	fmt.Println("Inode.CompressedCluster =", i.Flag.CompressedCluster)
	fmt.Println("Inode.DirHashIndexes    =", i.Flag.DirHashIndexes)
	fmt.Println("Inode.HugeFile          =", i.Flag.HugeFile)
	fmt.Println("Inode.UsesExtents       =", i.Flag.UsesExtents)

	// Parse the inode depending whether it is stored as extents or blocks
	if i.Flag.UsesExtents && fs.Extents {
		var size uint64
		size = i.DataSize
		if !ParseExtentHeader(i.Data[40:100], inodeType, &size) {
			fmt.Println("Error parsing inode extent header")
			return false
		}
		return true
	} else {
		if !ParseInodeBlockPointers(&i, inodeType) {
			fmt.Println("Error parsing inode block pointers")
			return false
		}
		return true
	}

	return false
}

// Parse directory data block
func ParseDirBlock(bdata []byte, size uint64) {
	fmt.Println("ParseDirBlock")

	for c := uint32(0); c < uint32(fs.BlockSize); {
		DirList[DirIndex].Id = DirIndex
		DirList[DirIndex].InodeNumber = uint64(ToInt(bdata[c+0 : c+4]))
		DirList[DirIndex].EntrySize = ToInt(bdata[c+4 : c+6])
		if fs.TypeInDirEntry {
			DirList[DirIndex].NameLength = ToInt(bdata[c+6 : c+7])
			DirList[DirIndex].Type = ToInt(bdata[c+7 : c+8])
		} else {
			// TO DO : If type not set in directory handle in inode
			DirList[DirIndex].NameLength = ToInt(bdata[c+6 : c+8])
			DirList[DirIndex].Type = 0
		}
		// Name from (c + 8) to (c + 8 + N - 1)
		// Making a copy of slice
		DirList[DirIndex].Name = make([]byte, DirList[DirIndex].NameLength)
		copy(DirList[DirIndex].Name, bdata[c+8:c+8+DirList[DirIndex].NameLength])

		//fmt.Println("c =", c, "index =", DirIndex, "entrysize =",
		//		DirList[DirIndex].EntrySize, "namelength =",
		//		DirList[DirIndex].NameLength, "name =",
		//		DirList[DirIndex].Name)
		c += DirList[DirIndex].EntrySize
		DirIndex++
	}
}

// Print directory listing from DirList array
func PrintDirList() {
	// Listing the directory contents
	fmt.Printf("ID  |INODE       |TYPE          |NAME\n")
	fmt.Printf("----|------------|--------------|-----------------\n")
	for _, dItem := range DirList {
		if dItem.InodeNumber == 0 {
			break
		}
		fmt.Printf("%3d | %10d | ", dItem.Id, dItem.InodeNumber)
		switch dItem.Type {
		case 0:
			fmt.Printf("Unknown      | ")
		case 1:
			fmt.Printf("File         | ")
		case 2:
			fmt.Printf("Directory    | ")
		case 3:
			fmt.Printf("Char device  | ")
		case 4:
			fmt.Printf("Block device | ")
		case 5:
			fmt.Printf("FIFO         | ")
		case 6:
			fmt.Printf("Socket       | ")
		case 7:
			fmt.Printf("Soft link    | ")
		}
		fmt.Printf("%s", dItem.Name)
		fmt.Printf("\n")
	}
}

// Parse file data block
func ParseFileBlock(bdata []byte, size uint64) {
	// Write data to output file
	// If size is less than BlockSize then write upto size else write whole block
	if size <= fs.BlockSize { // To Do : Can use only <
		dumpFile.Write(bdata[:size])
	} else {
		dumpFile.Write(bdata[:])
	}
}

// Read a block of data
func ReadBlock(blockNo uint64, n *Nucdp) bool {
	var offset uint64
	offset = blockNo * fs.BlockSize

	fmt.Println("ReadBlock : BlockNo =", blockNo, " ByteOffset =", offset)

	// Read block from file or database
	if DataSrc == "file" {
		devFile.Seek(int64(offset), 0)
		_, err := devFile.Read(n.Data)
		if err != nil {
			fmt.Println("Error reading block", err)
			return false
		}
	} else {
		row := db.QueryRow("SELECT n, b FROM `map`"+
			" WHERE n=? LIMIT 1", blockNo)

		var no, bid uint64

		err := row.Scan(&no, &bid)
		if err != nil {
			fmt.Println("Error fetching row", err.Error())
			return false
		}

		row = db.QueryRow("SELECT `b` FROM `2f6465762f73646131`"+
			" WHERE i=? LIMIT 1", bid)

		err = row.Scan(&n.Data)
		if err != nil {
			fmt.Println("Error fetching row", err.Error())
			return false
		}
	}
	return true
}

// Converting byte array to integer with little endian support
func ToInt(in []byte) uint32 {
	var out uint32 = 0
	for i := len(in) - 1; i >= 0; i-- {
		out = (out << 8) | uint32(in[i])
	}
	return out
}

// Check if the block contains backup copy of super block
// Backup copy of super block located at block number
// 0, 1 and power of 3, 5, 7
func BlockContainsSuper(blockNum uint64) bool {
	b := blockNum
	if b == 0 || b == 1 {
		return true
	}

	// Check if power of 3
	for (b % 3) == 0 {
		b = b / 3
	}
	if b == 1 {
		return true
	}

	// Check if power of 5
	b = blockNum
	for (b % 5) == 0 {
		b = b / 5
	}
	if b == 1 {
		return true
	}

	// Check if power of 7
	b = blockNum
	for (b % 7) == 0 {
		b = b / 7
	}
	if b == 1 {
		return true
	}
	return false
}

/////////////////// BLOCKS //////////////////////

func ParseInodeBlockPointers(i *Inode, inodeType uint32) bool {
	var c, c1, c2 int
	var n, n1, n2, n3 Nucdp
	var blockPtr uint64

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
		n1.Data = make([]byte, fs.BlockSize)
		n2.Data = make([]byte, fs.BlockSize)
		n3.Data = make([]byte, fs.BlockSize)
	}

	// Parse the data block pointers
	i.Direct[0] = uint64(ToInt(i.Data[40:44]))
	i.Direct[1] = uint64(ToInt(i.Data[44:48]))
	i.Direct[2] = uint64(ToInt(i.Data[48:52]))
	i.Direct[3] = uint64(ToInt(i.Data[52:56]))
	i.Direct[4] = uint64(ToInt(i.Data[56:60]))
	i.Direct[5] = uint64(ToInt(i.Data[60:64]))
	i.Direct[6] = uint64(ToInt(i.Data[64:68]))
	i.Direct[7] = uint64(ToInt(i.Data[68:72]))
	i.Direct[8] = uint64(ToInt(i.Data[72:76]))
	i.Direct[9] = uint64(ToInt(i.Data[76:80]))
	i.Direct[10] = uint64(ToInt(i.Data[80:84]))
	i.Direct[11] = uint64(ToInt(i.Data[84:88]))
	i.SingleIndirect = uint64(ToInt(i.Data[88:92]))
	i.DoubleIndirect = uint64(ToInt(i.Data[92:96]))
	i.TripeIndirect = uint64(ToInt(i.Data[96:100]))
	// fmt.Println("Direct0 =", i.Direct[0])

	var readBytes uint64 // total number of bytes read
	readBytes = 0

	// Read the direct block pointers
	fmt.Println("Start direct blocks")
	for c = 0; c < 12; c++ {
		if readBytes >= i.DataSize {
			return true
		}
		// fmt.Println(i.Direct[c], i.DataSize)
		if !ReadBlock(i.Direct[c], &n) {
			fmt.Println("Error reading direct block")
			return false
		}
		if inodeType == FileT { // File
			ParseFileBlock(n.Data, i.DataSize-readBytes)
		} else { // Directory
			ParseDirBlock(n.Data, i.DataSize-readBytes)
		}
		readBytes += fs.BlockSize
	}
	if readBytes >= i.DataSize {
		return true
	}

	// Read single indirect block
	fmt.Println("Start single indirect block")
	if !ReadBlock(i.SingleIndirect, &n) {
		fmt.Println("Error reading single indirect level 0 block")
		return false
	}
	// Parse each entry of the block - points to blocks that contains list of block pointers
	for c = 0; c < int(fs.BlockSize); c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = uint64(ToInt(n.Data[c : c+4]))
		// fmt.Println(blockPtr)
		// Final data blocks
		if !ReadBlock(blockPtr, &n1) {
			fmt.Println("Error reading single indirect level 1 block")
			return false
		}
		if inodeType == FileT { // File
			ParseFileBlock(n1.Data, i.DataSize-readBytes)
		} else { // Directory
			ParseDirBlock(n1.Data, i.DataSize-readBytes)
		}
		readBytes += fs.BlockSize
	}
	if readBytes >= i.DataSize {
		return true
	}

	// Read double indirect block
	fmt.Println("Start double indirect block")
	if !ReadBlock(i.DoubleIndirect, &n) {
		fmt.Println("Error reading double indirect level 0 block")
		return false
	}
	// Parse each entry of the block - points to blocks that contains list of block pointers
	for c = 0; c < int(fs.BlockSize); c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = uint64(ToInt(n.Data[c : c+4]))
		// fmt.Println(blockPtr)
		if !ReadBlock(blockPtr, &n1) {
			fmt.Println("Error reading double indirect level 1 block")
			return false
		}
		// Parse each entry of the block - points to blocks that contains list of block pointers
		for c1 = 0; c1 < int(fs.BlockSize); c1 += 4 {
			if readBytes >= i.DataSize {
				return true
			}
			blockPtr = uint64(ToInt(n1.Data[c1 : c1+4]))
			// fmt.Println(blockPtr)
			// Final data blocks
			if !ReadBlock(blockPtr, &n2) {
				fmt.Println("Error reading double indirect level 2 block")
				return false
			}
			if inodeType == FileT { // File
				ParseFileBlock(n2.Data, i.DataSize-readBytes)
			} else { // Directory
				ParseDirBlock(n2.Data, i.DataSize-readBytes)
			}
			readBytes += fs.BlockSize
		}
	}
	if readBytes >= i.DataSize {
		return true
	}

	// Read triple indirect block
	fmt.Println("Start triple indirect block")
	if !ReadBlock(i.TripeIndirect, &n) {
		fmt.Println("Error reading triple indirect level 0 block")
		return false
	}
	// Parse each entry of the block - points to blocks that contains list of block pointers
	for c = 0; c < int(fs.BlockSize); c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = uint64(ToInt(n.Data[c : c+4]))
		// fmt.Println(blockPtr)
		if !ReadBlock(blockPtr, &n1) {
			fmt.Println("Error reading triple indirect level 1 block")
			return false
		}
		// Parse each entry of the block - points to blocks that contains list of block pointers
		for c1 = 0; c1 < int(fs.BlockSize); c1 += 4 {
			if readBytes >= i.DataSize {
				return true
			}
			blockPtr = uint64(ToInt(n1.Data[c1 : c1+4]))
			// fmt.Println(blockPtr)
			if !ReadBlock(blockPtr, &n2) {
				fmt.Println("Error reading triple indirect level 2 block")
				return false
			}
			// Parse each entry of the block - points to blocks that contains list of block pointers
			for c2 = 0; c2 < int(fs.BlockSize); c2 += 4 {
				if readBytes >= i.DataSize {
					return true
				}
				blockPtr = uint64(ToInt(n2.Data[c2 : c2+4]))
				// fmt.Println(blockPtr)
				// Final data blocks
				if !ReadBlock(blockPtr, &n3) {
					fmt.Println("Error reading triple indirect level 3 block")
					return false
				}
				if inodeType == FileT { // File
					ParseFileBlock(n3.Data, i.DataSize-readBytes)
				} else { // Directory
					ParseDirBlock(n3.Data, i.DataSize-readBytes)
				}
				readBytes += fs.BlockSize
			}
		}
	}
	if readBytes >= i.DataSize {
		return true
	}
	fmt.Println("Beyond the size of file system")
	return false
}

/////////////////// EXTENTS /////////////////////

func ParseExtentHeader(data []byte, inodeType uint32, size *uint64) bool {
	var eh ExtentHeader
	var offset uint32

	eh.Magic = ToInt(data[0:2]) // TO DO Check header
	eh.Entries = ToInt(data[2:4])
	eh.Max = ToInt(data[4:6])
	eh.Depth = ToInt(data[6:8])
	eh.Generation = ToInt(data[8:12])

	fmt.Println("ExtentHeader ", eh, "inodeType ", inodeType, "size", *size)

	// if depth is 0 then this is a leaf node that contains extents pointing to data
	// else this is a index node that contains extents that points to extent headers
	if eh.Depth == 0 {
		offset = 12
		for c := uint32(0); c < eh.Entries; c++ {
			if !ParseExtent(data[offset:offset+12], inodeType, size) {
				fmt.Println("Error reading Extent")
				return false
			}
			offset += 12
		}
	} else {
		offset = 12
		for c := uint32(0); c < eh.Entries; c++ {
			if !ParseExtentIdx(data[offset:offset+12], inodeType, size) {
				fmt.Println("Error reading ExtentIndex")
				return false
			}
			offset += 12
		}
	}
	return true
}

func ParseExtentIdx(data []byte, inodeType uint32, size *uint64) bool {
	var n Nucdp
	var ei ExtentIdx
	var highValue, lowValue uint64

	ei.FileBlock = ToInt(data[0:4])
	highValue = uint64(ToInt(data[8:10]))
	lowValue = uint64(ToInt(data[4:8]))
	ei.Block = (highValue << 32) | lowValue

	fmt.Println("ExtentIdx ", ei, "inodeType ", inodeType, "size", *size)

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
	}

	if !ReadBlock(ei.Block, &n) {
		fmt.Println("Error reading ExtentIndex")
		return false
	}

	if !ParseExtentHeader(n.Data, inodeType, size) {
		fmt.Println("Error reading ParseExtentHeader")
		return false
	}

	return true
}

func ParseExtent(data []byte, inodeType uint32, size *uint64) bool {
	var ee Extent
	var highValue, lowValue uint64
	var n Nucdp

	ee.FileBlock = ToInt(data[0:4])
	ee.Len = ToInt(data[4:6])
	highValue = uint64(ToInt(data[6:8]))
	lowValue = uint64(ToInt(data[8:12]))
	ee.Block = (highValue << 32) | lowValue

	fmt.Println("Extent ", ee, "inodeType ", inodeType, "size", *size)

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
	}

	if inodeType == FileT { // File
		for c := uint64(0); c < uint64(ee.Len); c++ {
			if !ReadBlock(ee.Block+c, &n) {
				fmt.Println("Error reading FileBlock")
				return false
			}
			ParseFileBlock(n.Data, *size)
			*size = *size - fs.BlockSize
		}
	} else { // Directory
		for c := uint64(0); c < uint64(ee.Len); c++ {
			if !ReadBlock(ee.Block+c, &n) {
				fmt.Println("Error reading DirBlock")
				return false
			}
			ParseDirBlock(n.Data, *size)
			*size = *size - fs.BlockSize
		}
	}

	return true
}
