// Package implements browsing ext3 filesystem from disk or database
// Author : Prashant Shah <pshah.mumbai@gmail.com>
package main

// To Do :
// Fragments - Not implemented
// Type in Directory Entry
// Inode Metadata - permissions, user, group id, etc
// Direcory Hash
// ASF Directory
// Sparse file : $truncate -S 1G <filename>; $ls -s <filename>

import (
	_ "../../server/src/db"
	"database/sql"
	"fmt"
	"os"
	// _ "github.com/ziutek/mymysql/godrv"
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
	TotalInode          uint32 // Total number of inodes
	TotalBlocks         uint32 // Total number of blocks
	BlockSize           uint32 // Block size
	BlocksPerBlkGroup   uint32 // Blocks per block group
	InodesPerBlkGroup   uint32 // Inodes per block group
	InodeSize           uint32 // Inode size
	TotalBlockGroups    uint32 // Total number of block groups
	StartBGDT           uint32 // Start of Block Group Descriptor Table
	BlockGroupDescSize  uint32 // Size of each block group descriptor
	BlockDescPerBlock   uint32 // Block descriptors per block
	FeaturesPerformance uint32 // Flags that can be ingnored
	FeaturesReadWrite   uint32 // Flags required to read and write
	FeaturesRead        uint32 // Flags required to write, if not present can mount read-only
	Use64BitSize        bool   // Size of the 'file size' field
	TypeInDirEntry      bool   // Store the type of inodes in the directory entry itself
	Signature           uint32 // File system signature
	MajorVersion        uint32 // Major version
	MinorVersion        uint32 // Minor version
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
type BlockGrDesc struct {
	BlockBitmap uint32 // Starting block for the block bitmap - 1 block size
	InodeBitmap uint32 // Starting block for the inode bitmap - 1 block size
	InodeTable  uint32 // Starting block for the inode table
}

// Block Group Descriptor Map
var BlockGrDescMap []BlockGrDesc

// Inode
type Inode struct {
	Direct         [12]uint32 // Direct mapping
	SingleIndirect uint32     // Single indirect mapping
	DoubleIndirect uint32     // Double indirect mapping
	TripleIndirect uint32     // Tripple indirect mapping
	DataSize       uint64     // Size of the data pointed by the inode
	SectorCount    uint32     // Number of disk sector used
}

// Single Directory entry
type DirectoryEntry struct {
	Id          uint32 // Id number used in selection
	InodeNumber uint32 // Inode number of the entry
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

var devFile *os.File  // Block Device file
var dumpFile *os.File // Dump the restored file data into this file

var DataSrc string // Data source can be "file" or "db"

func main() {
	var err error

	DataSrc = "file" // "db" or "file"

	if DataSrc == "file" {
		// Open block device
		devFile, err = os.Open("/dev/sda3")
		if err != nil {
			fmt.Println("Error opening partition", err)
			return
		}
		defer devFile.Close()
	} else {
		// Open database connection
		// db, err = sql.Open("mymysql", fmt.Sprintf("%s/%s/%s", DB_NAME, DB_USER, DB_PASS))
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

	var input int   // Read user input buffer
	var isFile bool // Whether input selected is a file or not
	for {
		// Read user input
		fmt.Printf("Enter your choice (-1 to exit): ")
		fmt.Scanf("%d", &input)
		if input == -1 {
			break
		}
		// Check if the file type is directory or file
		if !(DirList[input].Type == FileT || DirList[input].Type == DirT) {
			fmt.Printf("File type not supported %d\n", DirList[input].Type)
			continue
		}
		if DirList[input].Type == FileT {
			isFile = true
		} else {
			isFile = false
		}
		// If file then open a file for dumping data with the same name
		if isFile {
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
		// If its a file then close the file after dumping data
		if isFile {
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

	// Read the super block
	if !ReadBlock(0, &n) {
		fmt.Println("Error reading super block")
		return false
	}

	// Parse file system information
	b := 1024 // Super block is located at 1024 offset of the 0th block
	fs.TotalInode = ToInt(n.Data[b : b+4])
	fs.TotalBlocks = ToInt(n.Data[b+4 : b+8])
	fs.BlockSize = ToInt(n.Data[b+24 : b+28])
	fs.BlockSize = 1024 << fs.BlockSize // Converting from log to decimal
	fs.BlocksPerBlkGroup = ToInt(n.Data[b+32 : b+36])
	fs.InodesPerBlkGroup = ToInt(n.Data[b+40 : b+44])
	fs.FeaturesPerformance = ToInt(n.Data[b+92 : b+96])
	fs.FeaturesReadWrite = ToInt(n.Data[b+96 : b+100])
	fs.FeaturesRead = ToInt(n.Data[b+100 : b+104])
	fs.Signature = ToInt(n.Data[b+56 : b+58])
	fs.MajorVersion = ToInt(n.Data[b+76 : b+80])
	fs.MinorVersion = ToInt(n.Data[b+62 : b+64])

	// Check if its a valid ext partition
	if fs.Signature != 0xEF53 {
		fmt.Println("Not a valid ext2/ext3/ext4 file system")
		return false
	}

	// If version < 1.0 then inode size fixed as 128
	if fs.MajorVersion < 1 {
		fs.InodeSize = 128
	} else {
		fs.InodeSize = ToInt(n.Data[b+88 : b+92])
	}

	// Calculate the total number of block groups
	fs.TotalBlockGroups = fs.TotalBlocks / fs.BlocksPerBlkGroup
	if (fs.TotalBlocks % fs.BlocksPerBlkGroup) != 0 {
		fs.TotalBlockGroups++
	}

	// Calculate the start block number of Block Group Descriptor Table
	if fs.BlockSize <= 1024 {
		fs.StartBGDT = 2
	} else {
		fs.StartBGDT = 1
	}

	// Calculate size of block group descriptor and number of descriptor
	// block in one block of file system
	fs.BlockGroupDescSize = 32
	fs.BlockDescPerBlock = fs.BlockSize / fs.BlockGroupDescSize

	// Check if the file size in inode is 32 or 64 bit
	fs.Use64BitSize = false
	if fs.MajorVersion >= 1 {
		if (fs.FeaturesRead & (1 << 1)) != 0 { // 2nd bit
			fs.Use64BitSize = true
		}
	}

	// Check if directory entries contains file type field
	fs.TypeInDirEntry = false
	if fs.MajorVersion >= 1 {
		if (fs.FeaturesReadWrite & (1 << 1)) != 0 { // 2nd bit
			fs.TypeInDirEntry = true
		}
	}

	fmt.Println("Total Inode  : ", fs.TotalInode)
	fmt.Println("Total Blocks : ", fs.TotalBlocks)
	fmt.Println("Block Size   : ", fs.BlockSize)
	fmt.Println("BlkPerBlkGrp : ", fs.BlocksPerBlkGroup)
	fmt.Println("InodePerBlkG : ", fs.InodesPerBlkGroup)
	fmt.Println("InodeSize    : ", fs.InodeSize)
	fmt.Println("TotalBlkGrp  : ", fs.TotalBlockGroups)
	fmt.Println("BlkGDesSize  : ", fs.BlockGroupDescSize)
	fmt.Println("BlkGDesPrBlk : ", fs.BlockDescPerBlock)
	fmt.Println("FeatPerf     : ", fs.FeaturesPerformance)
	fmt.Println("FeatRdWr     : ", fs.FeaturesReadWrite)
	fmt.Println("FeatRead     : ", fs.FeaturesRead)
	fmt.Println("Use64BitSize : ", fs.Use64BitSize)
	fmt.Println("TypeInDir    : ", fs.TypeInDirEntry)
	fmt.Println("Signature    : ", fs.Signature)
	fmt.Println("MajorVersion : ", fs.MajorVersion)
	fmt.Println("MinorVersion : ", fs.MinorVersion)

	return true
}

// Parse Block Group Descriptor Table
func ParseBGDT() bool {
	var n Nucdp
	var offset uint32       // Current byte offset of the descriptor entry
	var count uint32        // Current count of the descriptor entry within	a block
	var currentBlock uint32 // Current file system block containing the BGDT entries

	// Start with the first block of the Block Group Descriptor Table
	currentBlock = fs.StartBGDT

	// Create a map of BlockGrDesc with size of the total number of block groups
	BlockGrDescMap = make([]BlockGrDesc, fs.TotalBlockGroups)

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
	}

	// Process each BGDT Entries
	count = 0
	for i := uint32(0); i < fs.TotalBlockGroups; i++ {
		// Read block containing BGDT entries when count = 0
		if count == 0 {
			if !ReadBlock(currentBlock, &n) { // Read file system block
				fmt.Println("Error reading block group descriptor")
				return false
			}
			currentBlock++
		}

		offset = count * fs.BlockGroupDescSize // Offset of each entry

		// Fetch addresses of block bitmap, inode bitmap and inode table
		BlockGrDescMap[i].BlockBitmap = ToInt(n.Data[offset : offset+4])
		BlockGrDescMap[i].InodeBitmap = ToInt(n.Data[offset+4 : offset+8])
		BlockGrDescMap[i].InodeTable = ToInt(n.Data[offset+8 : offset+12])

		count++

		// Reset counter when starting with the next block
		if count >= fs.BlockDescPerBlock {
			count = 0
		}
	}

	// Print the BlockGrDescMap map
	fmt.Println("Block Group Descriptor Map :")
	for c, v := range BlockGrDescMap {
		fmt.Println(c, v)
	}
	return true
}

// Read Inode
func ReadInode(inodeNum uint32, inodeType uint32) bool {
	var n, n1, n2, n3 Nucdp
	var i Inode
	var blockPtr uint32
	var c, c1, c2 uint32

	fmt.Println("ReadInode : InodeNumber =", inodeNum, " Type =", inodeType)

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

	if DataSrc == "file" {
		n.Data = make([]byte, fs.BlockSize)
		n1.Data = make([]byte, fs.BlockSize)
		n2.Data = make([]byte, fs.BlockSize)
		n3.Data = make([]byte, fs.BlockSize)
	}

	// Calculating inode table index
	curBlockGroup := (inodeNum - 1) / fs.InodesPerBlkGroup // Inode number starts from 1
	indexInodeTable := (inodeNum - 1) % fs.InodesPerBlkGroup

	// Calculating exact block containing the inode
	blockStartInodeTable := BlockGrDescMap[curBlockGroup].InodeTable
	blockOffset := (indexInodeTable * fs.InodeSize) / fs.BlockSize
	blockNum := blockStartInodeTable + blockOffset

	// Read block containing the inode
	if !ReadBlock(blockNum, &n) {
		fmt.Println("Error reading Inode")
		return false
	}

	// Inode offset
	inodeBlockOffset := (indexInodeTable * fs.InodeSize) % fs.BlockSize

	// fmt.Println(inodeNum, curBlockGroup, indexInodeTable, blockStartInodeTable, blockOffset, blockNum, inodeBlockOffset)
	// Read inode
	inode := n.Data[inodeBlockOffset : inodeBlockOffset+fs.InodeSize]

	// Parse the data block pointers
	i.Direct[0] = ToInt(inode[40:44])
	i.Direct[1] = ToInt(inode[44:48])
	i.Direct[2] = ToInt(inode[48:52])
	i.Direct[3] = ToInt(inode[52:56])
	i.Direct[4] = ToInt(inode[56:60])
	i.Direct[5] = ToInt(inode[60:64])
	i.Direct[6] = ToInt(inode[64:68])
	i.Direct[7] = ToInt(inode[68:72])
	i.Direct[8] = ToInt(inode[72:76])
	i.Direct[9] = ToInt(inode[76:80])
	i.Direct[10] = ToInt(inode[80:84])
	i.Direct[11] = ToInt(inode[84:88])
	i.SingleIndirect = ToInt(inode[88:92])
	i.DoubleIndirect = ToInt(inode[92:96])
	i.TripleIndirect = ToInt(inode[96:100])

	// Calculating size
	var lowSize, highSize, blockSize64 uint64
	if fs.Use64BitSize { // If using 64 bit size
		lowSize = uint64(ToInt(inode[4:8]))
		highSize = uint64(ToInt(inode[108:112]))
		if highSize > 0 {
			i.DataSize = (highSize << 32) | lowSize
		} else {
			i.DataSize = lowSize
		}
	} else {
		i.DataSize = uint64(ToInt(inode[4:8]))
	}

	blockSize64 = uint64(fs.BlockSize)
	fmt.Println("Datasize =", i.DataSize)

	// Calculate sector count
	i.SectorCount = ToInt(inode[28:31])
	fmt.Println("SectorCount =", i.SectorCount)

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
		readBytes += blockSize64
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
	for c = uint32(0); c < fs.BlockSize; c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = ToInt(n.Data[c : c+4])
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
		readBytes += blockSize64
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
	for c = 0; c < fs.BlockSize; c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = ToInt(n.Data[c : c+4])
		// fmt.Println(blockPtr)
		if !ReadBlock(blockPtr, &n1) {
			fmt.Println("Error reading double indirect level 1 block")
			return false
		}
		// Parse each entry of the block - points to blocks that contains list of block pointers
		for c1 = 0; c1 < fs.BlockSize; c1 += 4 {
			if readBytes >= i.DataSize {
				return true
			}
			blockPtr = ToInt(n1.Data[c1 : c1+4])
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
			readBytes += blockSize64
		}
	}
	if readBytes >= i.DataSize {
		return true
	}

	// Read triple indirect block
	fmt.Println("Start triple indirect block")
	if !ReadBlock(i.TripleIndirect, &n) {
		fmt.Println("Error reading triple indirect level 0 block")
		return false
	}
	// Parse each entry of the block - points to blocks that contains list of block pointers
	for c = 0; c < fs.BlockSize; c += 4 {
		if readBytes >= i.DataSize {
			return true
		}
		blockPtr = ToInt(n.Data[c : c+4])
		// fmt.Println(blockPtr)
		if !ReadBlock(blockPtr, &n1) {
			fmt.Println("Error reading triple indirect level 1 block")
			return false
		}
		// Parse each entry of the block - points to blocks that contains list of block pointers
		for c1 = 0; c1 < fs.BlockSize; c1 += 4 {
			if readBytes >= i.DataSize {
				return true
			}
			blockPtr = ToInt(n1.Data[c1 : c1+4])
			// fmt.Println(blockPtr)
			if !ReadBlock(blockPtr, &n2) {
				fmt.Println("Error reading triple indirect level 2 block")
				return false
			}
			// Parse each entry of the block - points to blocks that contains list of block pointers
			for c2 = 0; c2 < fs.BlockSize; c2 += 4 {
				if readBytes >= i.DataSize {
					return true
				}
				blockPtr = ToInt(n2.Data[c2 : c2+4])
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
				readBytes += blockSize64
			}
		}
	}
	if readBytes >= i.DataSize {
		return true
	}
	fmt.Println("Beyond the size of file system")
	return false
}

// Parse directory data block that contains the list of entries within the directory
func ParseDirBlock(bdata []byte, size uint64) {
	fmt.Println("[ParseDirBlock]")

	for c := uint32(0); c < fs.BlockSize; {
		DirList[DirIndex].Id = DirIndex
		DirList[DirIndex].InodeNumber = ToInt(bdata[c+0 : c+4])
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
	if size <= uint64(fs.BlockSize) {
		dumpFile.Write(bdata[:size])
	} else {
		dumpFile.Write(bdata[:])
	}
}

// Read a block of data
func ReadBlock(blockNo uint32, n *Nucdp) bool {
	var offset int64
	offset = int64(blockNo) * int64(fs.BlockSize)

	fmt.Println("[ReadBlock] BlockNo =", blockNo, " ByteOffset =", offset)

	// Read block from file or database
	if DataSrc == "file" {
		devFile.Seek(offset, 0)
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
