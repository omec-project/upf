CXXFLAGS += -Werror=format-truncation -Warray-bounds -fbounds-check \
			-fno-strict-overflow -fno-delete-null-pointer-checks -fwrapv

# When doing performance analysis
#CXXFLAGS += -fno-omit-frame-pointer

$(info   CXXFLAGS is $(CXXFLAGS))
